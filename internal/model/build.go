package model

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"mime"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/imagemeta"
	"github.com/klppl/kvist/internal/markdown"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/publish"
	"github.com/klppl/kvist/internal/resolve"
	"github.com/klppl/kvist/internal/slug"
	"github.com/klppl/kvist/internal/source"
	"github.com/klppl/kvist/internal/vault"
)

// Warning codes produced while building the model.
const (
	WarnUnpublishedLink  = "unpublished_link"
	WarnUnpublishedEmbed = "unpublished_embed"
	WarnEmbedCycle       = "embed_cycle"
	WarnSiteConfig       = "site_config"
	WarnPermalink        = "permalink"
	WarnHome             = "home"
	WarnImage            = "image"
)

// BuildError lists the problems that make a build fail.
type BuildError struct{ Problems []string }

func (e *BuildError) Error() string {
	if len(e.Problems) == 1 {
		return e.Problems[0]
	}
	return fmt.Sprintf("%d problems:\n  %s", len(e.Problems), strings.Join(e.Problems, "\n  "))
}

// maxEmbedDepth bounds nested note embeds.
const maxEmbedDepth = 5

// maxNoteBytes bounds the size of a note read for the build.
const maxNoteBytes = 16 << 20

// reservedPrefixes are URL paths owned by generated pages.
var reservedPrefixes = []string{"/tags/", "/_assets/", "/_kvist/"}

var reservedFiles = []string{"/search-index.json", "/graph.json", "/index.xml", "/sitemap.xml", "/robots.txt", "/404.html"}

type noteState struct {
	note *Note
	file source.File
	src  []byte
	meta *vault.Meta
	doc  *markdown.Doc // for headings, block ids and sections
}

type builder struct {
	cfg      *config.Site
	rules    protocol.Rules
	snap     source.Snapshot
	hints    *source.Hints
	notes    map[string]*noteState // published notes by vault path
	assets   map[string]source.File
	used     map[string]bool // assets referenced by rendered content
	ix       *resolve.Index
	aliases  map[string][]string // lower(alias) → note paths
	warnings []protocol.Warning
	problems []string
	strict   bool // strict line breaks
}

// Build builds the content model from a snapshot. It reads every file it
// needs from the snapshot; notes that fail the publish rules are ignored.
func Build(site *config.Site, snap source.Snapshot) (*Site, error) {
	b := &builder{
		cfg:     site,
		rules:   site.Rules(),
		snap:    snap,
		hints:   snap.Hints(),
		notes:   map[string]*noteState{},
		assets:  map[string]source.File{},
		used:    map[string]bool{},
		aliases: map[string][]string{},
	}
	var siteToml, siteNote []byte
	for _, f := range snap.Files() {
		switch {
		case f.Path == protocol.SiteConfigPath:
			src, err := b.read(f)
			if err != nil {
				return nil, err
			}
			siteToml = src
		case f.Path == protocol.SiteNotePath:
			src, err := b.read(f)
			if err != nil {
				return nil, err
			}
			siteNote = src
		case protocol.IsReservedPath(f.Path):
		case protocol.IsNote(f.Path):
			src, err := b.read(f)
			if err != nil {
				return nil, err
			}
			d, meta := publish.EvaluateSource(b.rules, f.Path, src)
			if !d.Published {
				continue
			}
			b.notes[f.Path] = &noteState{file: f, src: src, meta: meta, doc: markdown.Parse(src[meta.BodyStart:])}
		case protocol.AllowedPath(f.Path, b.rules) && publish.AttachmentAllowed(b.rules, f.Path):
			b.assets[f.Path] = f
		}
	}

	paths := make([]string, 0, len(b.notes)+len(b.assets))
	for p := range b.notes {
		paths = append(paths, p)
	}
	for p := range b.assets {
		paths = append(paths, p)
	}
	b.ix = resolve.NewIndex(paths)

	s := &Site{ModelVersion: Version, Revision: snap.Revision(), BuiltAt: snap.Time().UTC(), notesBy: map[string]*Note{}}
	settings := b.readSettings(siteToml, siteNote)
	s.Config = b.siteConfig(settings)

	b.strict = s.Config.StrictLineBreaks
	b.makeNotes(s)
	b.checkURLs(s)
	if len(b.problems) > 0 {
		return nil, &BuildError{Problems: b.problems}
	}
	b.render(s)
	b.siteImages(s, settings)
	b.makeFolders(s)
	b.makeTags(s)
	b.makeAssets(s)
	b.makeGraph(s)
	b.finishSettings(s, settings)
	s.Warnings = b.warnings
	if len(b.problems) > 0 {
		return nil, &BuildError{Problems: b.problems}
	}
	return s, nil
}

func (b *builder) read(f source.File) ([]byte, error) {
	r, err := b.snap.Open(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Path, err)
	}
	defer r.Close()
	src, err := io.ReadAll(io.LimitReader(r, maxNoteBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Path, err)
	}
	if len(src) > maxNoteBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes", f.Path, maxNoteBytes)
	}
	return src, nil
}

func (b *builder) warn(code, p, format string, args ...any) {
	b.warnings = append(b.warnings, protocol.Warning{Code: code, Path: p, Message: fmt.Sprintf(format, args...)})
}

// --- site config ---

// siteConfig starts from the server's settings and applies the vault's
// (see settings.go).
func (b *builder) siteConfig(st siteSettings) SiteConfig {
	c := SiteConfig{
		Title:       b.cfg.Title,
		Description: b.cfg.Description,
		BaseURL:     strings.TrimRight(b.cfg.BaseURL, "/"),
		Language:    b.cfg.Language,
		Author:      b.cfg.Author,
		Params:      map[string]any{},

		StrictLineBreaks: b.cfg.StrictLineBreaks,
	}
	for k, v := range b.cfg.ThemeParams {
		c.Params[k] = v
	}
	renameGroups(c.Params)
	applySettings(&c, st)
	return c
}

// --- notes ---

func (b *builder) makeNotes(s *Site) {
	expose := map[string]bool{}
	for _, k := range b.cfg.Publish.ExposeFrontmatter {
		expose[k] = true
	}
	for p, ns := range b.notes {
		fm := ns.meta.Frontmatter
		n := &Note{Path: p, Aliases: ns.meta.Aliases}
		n.Title = firstString(fm, "title")
		if n.Title == "" {
			n.Title = ns.doc.Title()
		}
		if n.Title == "" {
			n.Title = strings.TrimSuffix(path.Base(p), path.Ext(p))
		}
		n.URL = b.noteURL(p, firstString(fm, "permalink"))
		n.Slug = strings.Trim(n.URL, "/")
		n.ID = n.Slug
		if n.ID == "" {
			n.ID = "index"
		}
		n.Updated = firstTime(fm, ns.file.MTime, "updated", "modified", "lastmod")
		n.Created = firstTime(fm, n.Updated, "created", "date")
		if n.Created.After(n.Updated) {
			n.Updated = n.Created
		}
		n.Description = firstString(fm, "description", "summary")
		if n.Description == "" {
			n.Description = truncate(ns.doc.FirstParagraph(), 200)
		}
		for k, v := range fm {
			if expose[k] {
				if n.Params == nil {
					n.Params = map[string]any{}
				}
				n.Params[k] = v
			}
		}
		if ref, ok := vault.ImageProperty(fm, vault.NoteImageKeys); ok {
			n.Image = b.imageURL(p, p, ref)
		}
		ns.doc.RemoveLeadingTitle(n.Title)
		n.TOC = toc(ns.doc.Headings)
		n.Features = ns.doc.Features
		for _, a := range n.Aliases {
			k := strings.ToLower(a)
			b.aliases[k] = append(b.aliases[k], p)
		}
		ns.note = n
		s.Notes = append(s.Notes, n)
		s.notesBy[p] = n
	}
	sort.Slice(s.Notes, func(i, j int) bool { return s.Notes[i].URL < s.Notes[j].URL })
}

// noteURL derives a note's URL: a sanitized permalink if given, else the
// slugified path. index.md and a note named like its folder ("A/A.md") are
// folder notes and get the folder's URL.
func (b *builder) noteURL(p, permalink string) string {
	if permalink != "" {
		u := "/" + slug.Path(strings.Trim(permalink, "/")) + "/"
		if u == "//" {
			u = "/"
		}
		if strings.Contains(permalink, "..") {
			b.warn(WarnPermalink, p, "permalink %q ignored", permalink)
		} else {
			return u
		}
	}
	dir, file := path.Split(p)
	stem := strings.TrimSuffix(file, path.Ext(file))
	dir = strings.TrimSuffix(dir, "/")
	if strings.EqualFold(stem, "index") || (dir != "" && stem == path.Base(dir)) {
		if dir == "" {
			return "/"
		}
		return "/" + slug.Path(dir) + "/"
	}
	u := slug.Path(p)
	if u == "" {
		u = slug.Make(fmt.Sprintf("note-%x", p))
	}
	return "/" + u + "/"
}

func (b *builder) checkURLs(s *Site) {
	byURL := map[string]*Note{}
	for _, n := range s.Notes {
		if other, ok := byURL[n.URL]; ok {
			b.problems = append(b.problems, fmt.Sprintf("%q and %q both want the URL %s; rename one or set a permalink", other.Path, n.Path, n.URL))
			continue
		}
		byURL[n.URL] = n
		for _, r := range reservedPrefixes {
			if strings.HasPrefix(n.URL, r) || n.URL+"/" == r || n.URL == r {
				b.problems = append(b.problems, fmt.Sprintf("%q would be published at %s, which is reserved; set a permalink", n.Path, n.URL))
			}
		}
		for _, r := range reservedFiles {
			if strings.TrimSuffix(n.URL, "/") == r {
				b.problems = append(b.problems, fmt.Sprintf("%q would be published at %s, which is reserved; set a permalink", n.Path, n.URL))
			}
		}
	}
}

func firstString(fm map[string]any, keys ...string) string {
	for _, k := range keys {
		for _, v := range vault.FrontmatterValue(fm, k) {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

var dateLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"}

func firstTime(fm map[string]any, fallback time.Time, keys ...string) time.Time {
	for _, k := range keys {
		for _, v := range vault.FrontmatterValue(fm, k) {
			switch v := v.(type) {
			case time.Time:
				return v.UTC()
			case string:
				for _, l := range dateLayouts {
					if t, err := time.Parse(l, strings.TrimSpace(v)); err == nil {
						return t.UTC()
					}
				}
			}
		}
	}
	return fallback.UTC()
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	cut := strings.LastIndexByte(string(r), ' ')
	if cut < len(string(r))/2 {
		cut = len(string(r))
	}
	return strings.TrimRight(string(r)[:cut], " ,.;:") + "…"
}

func toc(hs []markdown.Heading) []*Heading {
	var root []*Heading
	var stack []*Heading
	for _, h := range hs {
		e := &Heading{Level: h.Level, Text: h.Text, ID: h.ID}
		for len(stack) > 0 && stack[len(stack)-1].Level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			root = append(root, e)
		} else {
			p := stack[len(stack)-1]
			p.Children = append(p.Children, e)
		}
		stack = append(stack, e)
	}
	return root
}

// --- rendering and links ---

func (b *builder) render(s *Site) {
	backlinks := map[string][]*Backlink{}
	for _, n := range s.Notes {
		ns := b.notes[n.Path]
		r := &noteResolver{b: b, from: ns, stack: []string{n.Path}, top: true, root: n}
		ns.doc.Resolve(r)
		ns.doc.StrictLineBreaks = b.strict
		html, err := ns.doc.Render()
		if err != nil {
			b.problems = append(b.problems, fmt.Sprintf("%s: %v", n.Path, err))
			continue
		}
		n.Content = template.HTML(html)
		n.Text = ns.doc.PlainText()
		n.WordCount = len(strings.Fields(n.Text))
		n.ReadingTime = int(math.Ceil(float64(n.WordCount) / 200))
		if n.ReadingTime < 1 {
			n.ReadingTime = 1
		}
		n.TagNames = noteTags(b, ns)

		seen := map[string]bool{}
		for _, ref := range ns.doc.Refs() {
			if ref.Target.Kind != markdown.NoteTarget || ref.Target.Path == n.Path {
				continue
			}
			t := s.notesBy[ref.Target.Path]
			if t == nil || seen[t.Path] {
				continue
			}
			seen[t.Path] = true
			n.Links = append(n.Links, &Link{Target: t, TargetID: t.ID, Embed: ref.Embed})
			backlinks[t.Path] = append(backlinks[t.Path], &Backlink{Source: n, SourceID: n.ID, Context: ref.Context})
		}
	}
	for _, n := range s.Notes {
		bl := backlinks[n.Path]
		sort.SliceStable(bl, func(i, j int) bool { return bl[i].Source.URL < bl[j].Source.URL })
		n.Backlinks = bl
	}
}

// noteTags are the frontmatter tags plus the tags in the rendered body
// (not the ones in comments, which are never published), without the
// control tags.
func noteTags(b *builder, ns *noteState) []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		k := strings.ToLower(t)
		if seen[k] || b.isControlTag(t) {
			return
		}
		seen[k] = true
		out = append(out, t)
	}
	for _, t := range ns.meta.FrontmatterTags {
		add(t)
	}
	for _, t := range ns.doc.Tags() {
		add(t)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

func (b *builder) isControlTag(t string) bool {
	return strings.EqualFold(t, b.rules.PublicTag) || strings.EqualFold(t, b.rules.PrivateTag)
}

type noteResolver struct {
	b     *builder
	from  *noteState
	stack []string // embed chain, for cycle detection
	top   bool     // resolving the note itself, not an embed: report warnings
	root  *Note    // the page being rendered; embeds add their features to it
}

func (r *noteResolver) Resolve(ref markdown.Ref) markdown.Target {
	b := r.b
	var target string
	switch {
	case ref.Target == "":
		target = r.from.note.Path
	default:
		target = b.lookup(r.from.note.Path, ref)
	}
	if target == "" {
		r.report(ref)
		return markdown.Target{}
	}
	if ns, ok := b.notes[target]; ok {
		u := ns.note.URL
		if frag := fragment(ns.doc, ref.Subpath); frag != "" {
			u += "#" + frag
		}
		if target == r.from.note.Path && ref.Target == "" {
			u = "#" + fragment(ns.doc, ref.Subpath)
		}
		return markdown.Target{Kind: markdown.NoteTarget, URL: u, Path: target, Title: ns.note.Title}
	}
	if f, ok := b.assets[target]; ok {
		b.used[target] = true
		return markdown.Target{Kind: markdown.AssetTarget, URL: b.assetURL(f), Path: target, Media: mediaKind(target)}
	}
	r.report(ref)
	return markdown.Target{}
}

// lookup resolves a link target to a published path, or "". Client hints
// win when they point to a published note; a hint of null (the client saw a
// private target) leaves the link unresolved.
func (b *builder) lookup(from string, ref markdown.Ref) string {
	if !ref.Markdown && b.hints != nil {
		if hs, ok := b.hints.Notes[from]; ok {
			if h, ok := hs[ref.Target]; ok {
				if h == nil {
					return ""
				}
				if _, published := b.notes[*h]; published {
					return *h
				}
			}
		}
	}
	if p, ok := b.ix.Resolve(from, vault.Link{Target: ref.Target, Markdown: ref.Markdown}); ok {
		return p
	}
	if !ref.Markdown {
		if ps := b.aliases[strings.ToLower(ref.Target)]; len(ps) > 0 {
			sorted := append([]string(nil), ps...)
			sort.Strings(sorted)
			return sorted[0]
		}
	}
	return ""
}

func fragment(d *markdown.Doc, sub string) string {
	if sub == "" {
		return ""
	}
	if id, ok := strings.CutPrefix(sub, "^"); ok {
		if d.HasBlock(id) {
			return "^" + id
		}
		return ""
	}
	id, _ := d.HeadingID(sub)
	return id
}

func (r *noteResolver) report(ref markdown.Ref) {
	if !r.top {
		return
	}
	mode, code, what := r.b.cfg.Publish.UnpublishedLinks, WarnUnpublishedLink, "links to"
	if ref.Embed {
		mode, code, what = r.b.cfg.Publish.UnpublishedEmbeds, WarnUnpublishedEmbed, "embeds"
	}
	written := ref.Target
	if ref.Subpath != "" {
		written += "#" + ref.Subpath
	}
	switch mode {
	case config.LeakActionWarn:
		r.b.warn(code, r.from.note.Path, "%s %q, which is not published or does not exist; shown as plain text", what, written)
	case config.LeakActionError:
		r.b.problems = append(r.b.problems, fmt.Sprintf("%s %s %q, which is not published or does not exist", r.from.note.Path, what, written))
	}
}

func (r *noteResolver) Embed(t markdown.Target, ref markdown.Ref) (string, bool) {
	ns := r.b.notes[t.Path]
	for _, p := range r.stack {
		if p == t.Path {
			r.b.warn(WarnEmbedCycle, r.from.note.Path, "embed of %q would recurse; omitted", ref.Target)
			return "", false
		}
	}
	if len(r.stack) > maxEmbedDepth {
		r.b.warn(WarnEmbedCycle, r.from.note.Path, "embeds nested deeper than %d levels; omitted", maxEmbedDepth)
		return "", false
	}
	body := ns.src[ns.meta.BodyStart:]
	if ref.Subpath != "" {
		sec, ok := ns.doc.Section(ref.Subpath)
		if !ok {
			if r.top {
				r.b.warn(WarnUnpublishedEmbed, r.from.note.Path, "embeds %q, but that section does not exist; omitted", ref.Target+"#"+ref.Subpath)
			}
			return "", false
		}
		body = sec
	}
	doc := markdown.Parse(body)
	doc.StrictLineBreaks = r.b.strict
	doc.Resolve(&noteResolver{b: r.b, from: ns, stack: append(append([]string(nil), r.stack...), t.Path), root: r.root})
	r.root.Features.Math = r.root.Features.Math || doc.Features.Math
	r.root.Features.Mermaid = r.root.Features.Mermaid || doc.Features.Mermaid
	r.root.Features.Code = r.root.Features.Code || doc.Features.Code
	html, err := doc.Render()
	if err != nil {
		return "", false
	}
	return html, true
}

func (r *noteResolver) TagURL(name string) string {
	if r.b.isControlTag(name) {
		return ""
	}
	return tagURL(name)
}

func tagURL(name string) string { return "/tags/" + slug.Path(name) + "/" }

// --- assets ---

// imageURL resolves an image property (vault.ImageProperty) of the note
// from to a URL, and publishes the image it points to as if the note
// embedded it. An image that isn't a published attachment is left out
// with a warning naming file.
func (b *builder) imageURL(from, file string, ref vault.ImageRef) string {
	if ref.URL != "" {
		return ref.URL
	}
	target := b.lookup(from, markdown.Ref{Target: ref.Link.Target})
	f, ok := b.assets[target]
	if !ok || !vault.IsImage(target) {
		b.warn(WarnImage, file, "image %q is not a published image; left out", ref.Link.Target)
		return ""
	}
	b.used[target] = true
	return b.assetURL(f)
}

// assetURL is content-addressed. For images whose metadata is stripped the
// hash also covers that setting, so a file published unstripped is never
// reused as the stripped one (or the other way round).
func (b *builder) assetURL(f source.File) string {
	h := strings.TrimPrefix(f.Hash, protocol.HashPrefix)
	if strip := b.cfg.Publish.StripImageMetadata; strip != nil && *strip && imagemeta.Supported(path.Ext(f.Path)) {
		h = strings.TrimPrefix(protocol.HashBytes([]byte(f.Hash+":stripped")), protocol.HashPrefix)
	}
	if len(h) > 8 {
		h = h[:8]
	}
	base := path.Base(f.Path)
	ext := strings.ToLower(path.Ext(base))
	stem := slug.Make(strings.TrimSuffix(base, path.Ext(base)))
	if stem == "" {
		stem = "file"
	}
	return "/_assets/" + h + "/" + stem + ext
}

func mediaKind(p string) string {
	switch protocol.Ext(p) {
	case "png", "jpg", "jpeg", "gif", "webp", "svg", "avif", "bmp":
		return markdown.MediaImage
	case "mp3", "m4a", "ogg", "wav", "flac", "opus":
		return markdown.MediaAudio
	case "mp4", "webm", "mov", "ogv", "m4v":
		return markdown.MediaVideo
	case "pdf":
		return markdown.MediaPDF
	}
	return markdown.MediaOther
}

func (b *builder) makeAssets(s *Site) {
	for p := range b.used {
		f := b.assets[p]
		mt := mime.TypeByExtension(path.Ext(p))
		if mt == "" {
			mt = "application/octet-stream"
		}
		s.Assets = append(s.Assets, &Asset{Path: p, URL: b.assetURL(f), Size: f.Size, MediaType: mt, Hash: f.Hash})
	}
	sort.Slice(s.Assets, func(i, j int) bool { return s.Assets[i].URL < s.Assets[j].URL })
}

// --- folders, tags, graph, home ---

func (b *builder) makeFolders(s *Site) {
	s.Root = &Folder{Name: "", Path: "", URL: "/"}
	byPath := map[string]*Folder{"": s.Root}
	var get func(p string) *Folder
	get = func(p string) *Folder {
		if f, ok := byPath[p]; ok {
			return f
		}
		parent := get(dirOf(p))
		f := &Folder{Name: path.Base(p), Path: p, URL: "/" + slug.Path(p) + "/", Parent: parent}
		parent.Children = append(parent.Children, f)
		byPath[p] = f
		return f
	}
	for _, n := range s.Notes {
		f := get(dirOf(n.Path))
		n.Folder, n.FolderPath = f, f.Path
		f.Notes = append(f.Notes, n)
		if n.URL == f.URL {
			f.Index, f.IndexID = n, n.ID
		}
	}
	var finish func(f *Folder)
	finish = func(f *Folder) {
		sort.Slice(f.Children, func(i, j int) bool { return strings.ToLower(f.Children[i].Name) < strings.ToLower(f.Children[j].Name) })
		sort.Slice(f.Notes, func(i, j int) bool { return f.Notes[i].URL < f.Notes[j].URL })
		f.NoteIDs = make([]string, len(f.Notes))
		for i, n := range f.Notes {
			f.NoteIDs[i] = n.ID
		}
		for _, c := range f.Children {
			finish(c)
		}
	}
	finish(s.Root)
}

func dirOf(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}

func (b *builder) makeTags(s *Site) {
	byKey := map[string]*Tag{}
	var get func(name string) *Tag
	get = func(name string) *Tag {
		k := strings.ToLower(name)
		if t, ok := byKey[k]; ok {
			return t
		}
		t := &Tag{Name: name, Slug: slug.Path(name), URL: tagURL(name)}
		byKey[k] = t
		if i := strings.LastIndexByte(name, '/'); i > 0 {
			t.Parent = get(name[:i])
			t.Parent.Children = append(t.Parent.Children, t)
		} else {
			s.Tags = append(s.Tags, t)
		}
		return t
	}
	for _, n := range s.Notes {
		for _, name := range n.TagNames {
			t := get(name)
			n.Tags = append(n.Tags, t)
			for x := t; x != nil; x = x.Parent {
				if len(x.Notes) == 0 || x.Notes[len(x.Notes)-1] != n {
					x.Notes = append(x.Notes, n)
				}
			}
		}
	}
	for _, t := range byKey {
		sort.Slice(t.Notes, func(i, j int) bool { return t.Notes[i].URL < t.Notes[j].URL })
		t.NoteIDs = make([]string, len(t.Notes))
		for i, n := range t.Notes {
			t.NoteIDs[i] = n.ID
		}
		sort.Slice(t.Children, func(i, j int) bool { return strings.ToLower(t.Children[i].Name) < strings.ToLower(t.Children[j].Name) })
		s.AllTags = append(s.AllTags, t)
	}
	byName := func(ts []*Tag) {
		sort.Slice(ts, func(i, j int) bool { return strings.ToLower(ts[i].Name) < strings.ToLower(ts[j].Name) })
	}
	byName(s.Tags)
	byName(s.AllTags)
}

func (b *builder) makeGraph(s *Site) {
	s.Graph = Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Tags: []GraphTag{}}
	for _, t := range s.AllTags {
		s.Graph.Tags = append(s.Graph.Tags, GraphTag{Name: t.Name, URL: t.URL})
	}
	for _, n := range s.Notes {
		s.Graph.Nodes = append(s.Graph.Nodes, GraphNode{ID: n.ID, Title: n.Title, URL: n.URL, Tags: n.TagNames})
		for _, l := range n.Links {
			s.Graph.Edges = append(s.Graph.Edges, GraphEdge{Source: n.ID, Target: l.TargetID})
		}
	}
}
