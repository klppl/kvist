package model

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/vault"
)

// The vault may set presentation settings in two places: the settings note
// (_site.md, pushed as .kvist/site.md) and the older .kvist/site.toml. The
// note wins where both set something. Publish rules, the theme and the base
// URL stay in the server config.

// siteSettings are the vault's presentation settings, merged from both
// sources. Nil and empty fields leave the server's values alone.
type siteSettings struct {
	Title, Description, Author, Language *string
	StrictLineBreaks                     *bool
	Home                                 *string // vault path or [[wikilink]]
	HomeFrom                             string  // file that set Home, for warnings
	Nav                                  []navRef
	NavSet                               bool
	NavFrom                              string
	Params                               map[string]any
}

// navRef is a menu link: a URL, or a note to resolve once URLs are known.
type navRef struct {
	Title string
	URL   string      // set for links to URLs
	Note  *vault.Link // set for links to notes
}

// siteTomlSettings is the shape of .kvist/site.toml.
type siteTomlSettings struct {
	Title            *string        `toml:"title"`
	Description      *string        `toml:"description"`
	Author           *string        `toml:"author"`
	Language         *string        `toml:"language"`
	StrictLineBreaks *bool          `toml:"strict_line_breaks"`
	Home             *string        `toml:"home"`
	Nav              []NavItem      `toml:"nav"`
	ThemeParams      map[string]any `toml:"theme_params"`
}

// readSettings merges site.toml and then the settings note.
func (b *builder) readSettings(siteToml, siteNote []byte) siteSettings {
	st := siteSettings{Params: map[string]any{}}
	if siteToml != nil {
		b.readSiteToml(&st, siteToml)
	}
	if siteNote != nil {
		b.readSiteNote(&st, siteNote)
	}
	renameGroups(st.Params)
	return st
}

// renameGroups maps the old theme setting name nav_tags to groups, and
// turns "articles, projects" written as text into a list.
func renameGroups(params map[string]any) {
	if v, ok := params["nav_tags"]; ok {
		if _, set := params["groups"]; !set {
			params["groups"] = v
		}
		delete(params, "nav_tags")
	}
	if v, ok := params["groups"].(string); ok {
		var list []any
		for _, g := range strings.Split(v, ",") {
			if g = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(g), "#")); g != "" {
				list = append(list, g)
			}
		}
		params["groups"] = list
	}
	if v, ok := params["groups"].([]any); ok {
		list := make([]any, 0, len(v))
		for _, g := range v {
			if gs, ok := g.(string); ok {
				g = strings.TrimPrefix(strings.TrimSpace(gs), "#")
			}
			list = append(list, g)
		}
		params["groups"] = list
	}
}

func (b *builder) readSiteToml(st *siteSettings, src []byte) {
	var o siteTomlSettings
	md, err := toml.Decode(string(src), &o)
	if err != nil {
		b.warn(WarnSiteConfig, protocol.SiteConfigPath, "ignored: %v", err)
		return
	}
	for _, k := range md.Undecoded() {
		b.warn(WarnSiteConfig, protocol.SiteConfigPath, "key %q is not allowed in the vault and was ignored (it can only be set in the server config)", k.String())
	}
	st.Title, st.Description, st.Author, st.Language = o.Title, o.Description, o.Author, o.Language
	st.StrictLineBreaks = o.StrictLineBreaks
	if o.Home != nil {
		st.Home, st.HomeFrom = o.Home, protocol.SiteConfigPath
	}
	if o.Nav != nil {
		st.Nav, st.NavSet, st.NavFrom = nil, true, protocol.SiteConfigPath
		for _, n := range o.Nav {
			st.Nav = append(st.Nav, navRef{Title: n.Title, URL: n.URL})
		}
	}
	for k, v := range o.ThemeParams {
		st.Params[k] = v
	}
}

// serverOnlyKeys can't be set from the vault; the settings note warns
// instead of passing them to the theme.
var serverOnlyKeys = map[string]bool{
	"id": true, "base_url": true, "theme": true, "theme_overrides": true, "serve": true,
	"always_public_folders": true, "exclude_folders": true, "public_tag": true, "private_tag": true,
	"frontmatter_key": true, "attachment_extensions": true, "expose_frontmatter": true,
	"unpublished_links": true, "unpublished_embeds": true, "strip_image_metadata": true,
}

// ignoredKeys are Obsidian's own properties, not settings.
var ignoredKeys = map[string]bool{"tags": true, "tag": true, "aliases": true, "alias": true, "cssclasses": true, "cssclass": true, "publish": true}

func (b *builder) readSiteNote(st *siteSettings, src []byte) {
	m := vault.ParseMeta(src)
	if m.FrontmatterErr != nil {
		b.warn(WarnSiteConfig, protocol.SiteNotePath, "the properties could not be read and were ignored: %v", m.FrontmatterErr)
	}
	keys := make([]string, 0, len(m.Frontmatter))
	for k := range m.Frontmatter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m.Frontmatter[k]
		if isEmpty(v) {
			continue // an empty property means "not set", not "set to nothing"
		}
		lk := strings.ToLower(k)
		s := func() *string { x := strings.TrimSpace(scalar(v)); return &x }
		switch {
		case lk == "title":
			st.Title = s()
		case lk == "description":
			st.Description = s()
		case lk == "author":
			st.Author = s()
		case lk == "language":
			st.Language = s()
		case lk == "home":
			st.Home, st.HomeFrom = s(), protocol.SiteNotePath
		case lk == "strict_line_breaks":
			if on, ok := boolValue(v); ok {
				st.StrictLineBreaks = &on
			} else {
				b.warn(WarnSiteConfig, protocol.SiteNotePath, "%q should be true or false; ignored", k)
			}
		case ignoredKeys[lk]:
		case serverOnlyKeys[lk]:
			b.warn(WarnSiteConfig, protocol.SiteNotePath, "%q can only be set in the server config; ignored", k)
		default:
			st.Params[k] = v
		}
	}
	if nav := parseNavList(src[m.BodyStart:]); nav != nil {
		st.Nav, st.NavSet, st.NavFrom = nav, true, protocol.SiteNotePath
	}
}

// isEmpty reports whether a property was left blank.
func isEmpty(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	}
	return false
}

// boolValue reads a checkbox property (or "true"/"false" written as text).
func boolValue(v any) (bool, bool) {
	switch v := v.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "on":
			return true, true
		case "false", "no", "off":
			return false, true
		}
	}
	return false, false
}

// scalar renders a property value as text (YAML may give numbers or bools).
func scalar(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		if len(v) == 1 {
			return scalar(v[0])
		}
	}
	return fmt.Sprint(v)
}

var (
	navMarkdown = regexp.MustCompile(`^\s*[-*+]\s+\[([^\]]+)\]\(\s*<?([^)>\s]+)>?\s*\)\s*$`)
	navWiki     = regexp.MustCompile(`^\s*[-*+]\s+\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|([^\]]+))?\]\]\s*$`)
	hasScheme   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	comments    = regexp.MustCompile(`(?s)%%.*?%%|<!--.*?-->`)
)

// parseNavList reads menu links from the note body: every list item that
// is a single link, in order. Links to URLs (https:, mailto:, /about/) are
// kept as they are; [[wikilinks]] and links to .md files point at notes.
// Links in code blocks and comments don't count.
func parseNavList(body []byte) []navRef {
	var out []navRef
	inFence := false
	// Commented-out links (%% … %%, <!-- … -->) are examples, not links.
	text := comments.ReplaceAllString(string(body), "")
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := navWiki.FindStringSubmatch(line); m != nil {
			out = append(out, navRef{Title: strings.TrimSpace(m[2]), Note: &vault.Link{Target: strings.TrimSpace(m[1])}})
			continue
		}
		if m := navMarkdown.FindStringSubmatch(line); m != nil {
			title, target := strings.TrimSpace(m[1]), m[2]
			if hasScheme.MatchString(target) || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "#") {
				out = append(out, navRef{Title: title, URL: target})
				continue
			}
			if dec, err := url.PathUnescape(target); err == nil {
				target = dec
			}
			target, _, _ = strings.Cut(target, "#")
			out = append(out, navRef{Title: title, Note: &vault.Link{Target: target, Markdown: true}})
		}
	}
	return out
}

// applySettings fills the site config from the server's values and the
// vault's settings. Links to notes are resolved later, in finishSettings.
func applySettings(c *SiteConfig, st siteSettings) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&c.Title, st.Title)
	set(&c.Description, st.Description)
	set(&c.Author, st.Author)
	set(&c.Language, st.Language)
	if st.StrictLineBreaks != nil {
		c.StrictLineBreaks = *st.StrictLineBreaks
	}
	for k, v := range st.Params {
		c.Params[k] = v
	}
}

// finishSettings resolves the home page and the menu links once every
// published note has its URL. Links to notes that aren't published are
// dropped with a warning, so the menu never hints at a private note.
func (b *builder) finishSettings(s *Site, st siteSettings) {
	if root := s.Root; root != nil && root.Index != nil {
		s.Home, s.HomeID = root.Index, root.Index.ID
	}
	if st.Home != nil && strings.TrimSpace(*st.Home) != "" {
		ref := strings.TrimSpace(*st.Home)
		var n *Note
		if strings.HasPrefix(ref, "[[") && strings.HasSuffix(ref, "]]") {
			target := strings.TrimSuffix(strings.TrimPrefix(ref, "[["), "]]")
			target, _, _ = strings.Cut(target, "|")
			target, _, _ = strings.Cut(target, "#")
			n = b.noteFor(s, &vault.Link{Target: strings.TrimSpace(target)})
		} else {
			n = s.notesBy[protocol.NormalizePath(strings.TrimPrefix(ref, "/"))]
		}
		if n == nil {
			b.warn(WarnHome, st.HomeFrom, "home note %q is not published", ref)
		} else {
			s.Home, s.HomeID = n, n.ID
		}
	}
	if !st.NavSet {
		return
	}
	s.Config.Nav = nil
	for _, r := range st.Nav {
		if r.Note == nil {
			s.Config.Nav = append(s.Config.Nav, NavItem{Title: r.Title, URL: r.URL})
			continue
		}
		n := b.noteFor(s, r.Note)
		if n == nil {
			b.warn(WarnSiteConfig, st.NavFrom, "menu link %q points to a note that is not published; left out", r.Note.Target)
			continue
		}
		title := r.Title
		if title == "" {
			title = n.Title
		}
		s.Config.Nav = append(s.Config.Nav, NavItem{Title: title, URL: n.URL})
	}
}

// noteFor resolves a link from the settings note to a published note.
func (b *builder) noteFor(s *Site, l *vault.Link) *Note {
	p, ok := b.ix.Resolve(protocol.SettingsNoteName, *l) // relative to the vault root
	if !ok {
		return nil
	}
	return s.notesBy[p]
}
