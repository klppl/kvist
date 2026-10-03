// Package vault extracts metadata from Obsidian notes: frontmatter, tags,
// aliases and outgoing links. It is pure (no I/O) and is used by the publish
// rules (gate 2), by the reference push client (gate 1) and by the build.
//
// The extractor works on the raw text rather than a Markdown AST so the
// publish decision is cheap and independent of rendering. Code (fenced,
// indented and inline) is ignored for tags and links. Comments (%%…%% and
// <!-- … -->) are ignored for links, because comment content is never
// rendered, but not for tags: Obsidian indexes tags written in comments, and
// people hide a #private tag in a comment to keep it out of the reading view.
package vault

import (
	"bytes"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Meta is the metadata of one note.
type Meta struct {
	// Frontmatter is the decoded YAML frontmatter; empty if there is none.
	Frontmatter map[string]any
	// FrontmatterErr is set when a frontmatter block exists but is not a
	// valid YAML mapping. Callers must fail closed on it.
	FrontmatterErr error
	// BodyStart is the byte offset where the body begins.
	BodyStart int
	// Tags lists tag names without '#', frontmatter tags first, then body tags
	// in order of appearance, deduplicated case-insensitively.
	Tags []string
	// FrontmatterTags are the tags from the frontmatter alone.
	FrontmatterTags []string
	// Aliases from the frontmatter `aliases`/`alias` property.
	Aliases []string
	// Links lists outgoing links and embeds outside code and comments.
	Links []Link
}

// Link is one outgoing link or embed.
type Link struct {
	Target   string // vault path or note name, decoded; "" for a same-note link
	Subpath  string // heading or "^block" after '#', without the '#'
	Alias    string // display text: wikilink alias or Markdown link text
	Embed    bool   // ![[…]] or ![…](…)
	Markdown bool   // [text](target) rather than [[target]]
	Offset   int    // byte offset of the link in the source
}

// HasTag reports whether the note carries tag (case-insensitive, exact).
func (m *Meta) HasTag(tag string) bool {
	for _, t := range m.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// ParseMeta extracts the metadata of a note.
func ParseMeta(src []byte) *Meta {
	m := &Meta{Frontmatter: map[string]any{}}
	fm, bodyStart, ok := splitFrontmatter(src)
	m.BodyStart = bodyStart
	if ok {
		m.parseFrontmatter(fm)
	}
	body := src[bodyStart:]
	codeMasked := maskCode(body)
	allMasked := maskComments(codeMasked)

	seen := map[string]bool{}
	addTag := func(t string) {
		k := strings.ToLower(t)
		if t == "" || seen[k] {
			return
		}
		seen[k] = true
		m.Tags = append(m.Tags, t)
	}
	for _, t := range frontmatterList(m.Frontmatter, "tags", "tag") {
		if t = strings.TrimPrefix(strings.TrimSpace(t), "#"); validTag(t) {
			m.FrontmatterTags = append(m.FrontmatterTags, t)
			addTag(t)
		}
	}
	for _, t := range scanTags(codeMasked) {
		addTag(t)
	}
	m.Aliases = frontmatterList(m.Frontmatter, "aliases", "alias")
	m.Links = scanLinks(allMasked, bodyStart)
	return m
}

// splitFrontmatter finds a leading "---" … "---" block. It returns the YAML
// text, the offset of the body and whether a block was found.
func splitFrontmatter(src []byte) ([]byte, int, bool) {
	start := 0
	if bytes.HasPrefix(src, []byte("\xef\xbb\xbf")) {
		start = 3
	}
	first, rest, ok := cutLine(src[start:])
	if !ok || strings.TrimRight(string(first), " \t\r") != "---" {
		return nil, 0, false
	}
	off := start + len(src[start:]) - len(rest)
	yamlStart := off
	for len(rest) > 0 {
		line, next, _ := cutLine(rest)
		lineStart := off
		off += len(rest) - len(next)
		t := strings.TrimRight(string(line), " \t\r")
		if t == "---" || t == "..." {
			return src[yamlStart:lineStart], off, true
		}
		rest = next
	}
	return nil, 0, false
}

// cutLine splits b after the first newline. ok is false only for empty input.
func cutLine(b []byte) (line, rest []byte, ok bool) {
	if len(b) == 0 {
		return nil, nil, false
	}
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[:i], b[i+1:], true
	}
	return b, nil, true
}

func (m *Meta) parseFrontmatter(fm []byte) {
	if len(bytes.TrimSpace(fm)) == 0 {
		return
	}
	var node yaml.Node
	if err := yaml.Unmarshal(fm, &node); err != nil {
		m.FrontmatterErr = err
		return
	}
	if len(node.Content) == 0 {
		return
	}
	if node.Content[0].Kind != yaml.MappingNode {
		m.FrontmatterErr = fmt.Errorf("frontmatter is not a mapping")
		return
	}
	var out map[string]any
	if err := node.Content[0].Decode(&out); err != nil {
		m.FrontmatterErr = err
		return
	}
	if out != nil {
		m.Frontmatter = out
	}
}

// FrontmatterValue looks up a key case-insensitively. If several keys match,
// all their values are returned in sorted key order.
func FrontmatterValue(fm map[string]any, key string) []any {
	var keys []string
	for k := range fm {
		if strings.EqualFold(k, key) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	vals := make([]any, len(keys))
	for i, k := range keys {
		vals[i] = fm[k]
	}
	return vals
}

// frontmatterList reads a list-like property: a YAML list of scalars, or a
// string separated by commas and/or whitespace.
func frontmatterList(fm map[string]any, keys ...string) []string {
	var out []string
	for _, key := range keys {
		for _, v := range FrontmatterValue(fm, key) {
			switch v := v.(type) {
			case string:
				if key == "aliases" || key == "alias" {
					for _, s := range strings.Split(v, ",") {
						if s = strings.TrimSpace(s); s != "" {
							out = append(out, s)
						}
					}
				} else {
					out = append(out, strings.FieldsFunc(v, func(r rune) bool {
						return r == ',' || unicode.IsSpace(r)
					})...)
				}
			case []any:
				for _, item := range v {
					switch item := item.(type) {
					case string:
						if item = strings.TrimSpace(item); item != "" {
							out = append(out, item)
						}
					case int, float64, bool:
						out = append(out, fmt.Sprint(item))
					}
				}
			}
		}
	}
	return out
}

// validTag applies Obsidian's tag syntax: letters, digits, marks, '_', '-'
// and '/', at least one character that is not a digit.
func validTag(t string) bool {
	if t == "" {
		return false
	}
	nonDigit := false
	for _, r := range t {
		if !isTagRune(r) {
			return false
		}
		if !unicode.IsDigit(r) {
			nonDigit = true
		}
	}
	return nonDigit
}

func isTagRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) ||
		r == '_' || r == '-' || r == '/'
}

// scanTags finds inline #tags in masked body text.
func scanTags(b []byte) []string {
	var tags []string
	for i := 0; i < len(b); i++ {
		if b[i] != '#' {
			continue
		}
		if i > 0 {
			prev, _ := utf8.DecodeLastRune(b[:i])
			if !unicode.IsSpace(prev) {
				continue
			}
		}
		j := i + 1
		for j < len(b) {
			r, size := utf8.DecodeRune(b[j:])
			if !isTagRune(r) {
				break
			}
			j += size
		}
		t := strings.TrimRight(string(b[i+1:j]), "/")
		if validTag(t) {
			tags = append(tags, t)
		}
		i = j - 1
	}
	return tags
}

// scanLinks finds wikilinks, embeds and Markdown links in masked body text.
func scanLinks(b []byte, base int) []Link {
	var links []Link
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' {
			i++ // escaped character
			continue
		}
		if b[i] != '[' {
			continue
		}
		embed := i > 0 && b[i-1] == '!'
		start := i
		if embed {
			start = i - 1
		}
		if i+1 < len(b) && b[i+1] == '[' {
			end := indexLineLocal(b[i+2:], "]]")
			if end < 0 {
				continue
			}
			inner := string(b[i+2 : i+2+end])
			l := parseWikilink(inner)
			l.Embed, l.Offset = embed, base+start
			if l.Target != "" || l.Subpath != "" {
				links = append(links, l)
			}
			i = i + 2 + end + 1
			continue
		}
		if l, next, ok := parseMarkdownLink(b, i); ok {
			l.Embed, l.Offset = embed, base+start
			links = append(links, l)
			i = next - 1
		}
	}
	return links
}

// indexLineLocal is bytes.Index limited to the current line.
func indexLineLocal(b []byte, sep string) int {
	if nl := bytes.IndexByte(b, '\n'); nl >= 0 {
		b = b[:nl]
	}
	return bytes.Index(b, []byte(sep))
}

func parseWikilink(inner string) Link {
	var l Link
	target, alias, _ := strings.Cut(inner, "|")
	l.Alias = strings.TrimSpace(alias)
	target, sub, _ := strings.Cut(target, "#")
	l.Target = strings.TrimSpace(target)
	l.Subpath = strings.TrimSpace(sub)
	return l
}

// parseMarkdownLink parses [text](dest "title") starting at b[i] == '['.
// It returns the link, the index after it and whether it matched. External
// URLs do not match.
func parseMarkdownLink(b []byte, i int) (Link, int, bool) {
	depth := 0
	j := i
	for ; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '[':
			depth++
		case ']':
			depth--
		case '\n':
			return Link{}, 0, false
		}
		if depth == 0 {
			break
		}
	}
	if j+1 >= len(b) || b[j] != ']' || b[j+1] != '(' {
		return Link{}, 0, false
	}
	text := string(b[i+1 : j])
	k := j + 2
	for k < len(b) && b[k] == ' ' {
		k++
	}
	var dest string
	if k < len(b) && b[k] == '<' {
		end := bytes.IndexByte(b[k:], '>')
		if end < 0 {
			return Link{}, 0, false
		}
		dest = string(b[k+1 : k+end])
		k += end + 1
	} else {
		parens := 0
		s := k
		for ; k < len(b); k++ {
			c := b[k]
			if c == '\\' {
				k++
				continue
			}
			if c == '(' {
				parens++
			} else if c == ')' {
				if parens == 0 {
					break
				}
				parens--
			} else if c == ' ' || c == '\n' {
				break
			}
		}
		dest = string(b[s:k])
	}
	// Skip an optional title, then expect ')'.
	end := bytes.IndexByte(b[k:], ')')
	if end < 0 || bytes.IndexByte(b[k:k+end], '\n') >= 0 {
		return Link{}, 0, false
	}
	next := k + end + 1
	if dest == "" || isExternal(dest) {
		return Link{}, 0, false
	}
	if u, err := url.PathUnescape(dest); err == nil {
		dest = u
	}
	target, sub, _ := strings.Cut(dest, "#")
	return Link{Target: target, Subpath: sub, Alias: text, Markdown: true}, next, true
}

func isExternal(dest string) bool {
	if strings.HasPrefix(dest, "//") {
		return true
	}
	colon := strings.IndexByte(dest, ':')
	if colon <= 0 {
		return false
	}
	for _, r := range dest[:colon] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}
