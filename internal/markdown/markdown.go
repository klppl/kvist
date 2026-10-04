package markdown

import (
	"net/url"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/klppl/kvist/internal/slug"
	"github.com/klppl/kvist/internal/vault"
)

// Heading is one heading of a note, for tables of contents and #links.
type Heading struct {
	Level int
	Text  string
	ID    string
}

// Features lists what a rendered note needs on the client.
type Features struct {
	Math    bool `json:"math"`
	Mermaid bool `json:"mermaid"`
	Code    bool `json:"code"`
}

var (
	mdOnce sync.Once
	md     goldmark.Markdown
)

func markdown() goldmark.Markdown {
	mdOnce.Do(func() {
		md = goldmark.New(
			goldmark.WithExtensions(extension.GFM, extension.Footnote),
			goldmark.WithParserOptions(
				parser.WithInlineParsers(inlineParsers()...),
				parser.WithBlockParsers(blockParsers()...),
				parser.WithASTTransformers(util.Prioritized(transformer{}, 100)),
			),
		)
	})
	return md
}

// Doc is a parsed note body.
type Doc struct {
	src      []byte
	root     ast.Node
	Headings []Heading
	Features Features
	blockIDs map[string]bool
	refs     []*refNode

	// StrictLineBreaks renders single line breaks as spaces (Obsidian's
	// "strict line breaks"); by default each one becomes <br>.
	StrictLineBreaks bool
}

// refNode is an internal link or embed in the document.
type refNode struct {
	node ast.Node
	Ref  Ref
	// Set by Resolve.
	Target  Target
	Context string
}

// Parse parses a note body (frontmatter already removed). Comments outside
// code are stripped before parsing, so their content never reaches the AST.
func Parse(body []byte) *Doc {
	src := append([]byte(nil), vault.StripComments(body)...)
	root := markdown().Parser().Parse(text.NewReader(src))
	d := &Doc{src: src, root: root, blockIDs: map[string]bool{}}
	var ids slug.Unique
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			h := Heading{Level: n.Level, Text: plainText(n, src)}
			h.ID = ids.Get(h.Text)
			n.SetAttributeString("id", []byte(h.ID))
			d.Headings = append(d.Headings, h)
		case *WikiLink:
			d.refs = append(d.refs, &refNode{node: n, Ref: Ref{Target: n.Target, Subpath: n.Subpath, Embed: n.Embed}})
		case *ast.Link:
			if ref, ok := internalRef(string(n.Destination), false); ok {
				d.refs = append(d.refs, &refNode{node: n, Ref: ref})
			}
		case *ast.Image:
			if ref, ok := internalRef(string(n.Destination), true); ok {
				d.refs = append(d.refs, &refNode{node: n, Ref: ref})
			}
		case *Math, *MathBlock:
			d.Features.Math = true
		case *Mermaid:
			d.Features.Mermaid = true
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			d.Features.Code = true
		}
		if id, ok := n.AttributeString("id"); ok {
			if b, ok := id.([]byte); ok && len(b) > 1 && b[0] == '^' {
				d.blockIDs[string(b[1:])] = true
			}
		}
		return ast.WalkContinue, nil
	})
	return d
}

// internalRef interprets a Markdown link destination. External URLs (with a
// scheme or starting with //) are not internal.
func internalRef(dest string, embed bool) (Ref, bool) {
	if dest == "" || isExternal(dest) {
		return Ref{}, false
	}
	if u, err := url.PathUnescape(dest); err == nil {
		dest = u
	}
	target, sub, _ := strings.Cut(dest, "#")
	return Ref{Target: target, Subpath: sub, Embed: embed, Markdown: true}, true
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

// Title returns the text of the first level-1 heading, if any.
func (d *Doc) Title() string {
	for _, h := range d.Headings {
		if h.Level == 1 {
			return h.Text
		}
	}
	return ""
}

// FirstParagraph returns the plain text of the first top-level paragraph.
func (d *Doc) FirstParagraph() string {
	for c := d.root.FirstChild(); c != nil; c = c.NextSibling() {
		if p, ok := c.(*ast.Paragraph); ok {
			if t := plainText(p, d.src); t != "" {
				return t
			}
		}
	}
	return ""
}

// PlainText returns the text content of the whole note (for search and
// word counts). Call it after Resolve so unresolved embeds are left out.
func (d *Doc) PlainText() string { return plainText(d.root, d.src) }

// HeadingID finds the anchor of a heading by its text, as written in a
// [[Note#Heading]] link: exact text (ignoring case and spacing) first, then
// by slug. For nested subpaths (#A#B) the last part counts.
func (d *Doc) HeadingID(sub string) (string, bool) {
	if i := strings.LastIndexByte(sub, '#'); i >= 0 {
		sub = sub[i+1:]
	}
	want := normalizeHeading(sub)
	for _, h := range d.Headings {
		if normalizeHeading(h.Text) == want {
			return h.ID, true
		}
	}
	ws := slug.Make(sub)
	for _, h := range d.Headings {
		if slug.Make(h.Text) == ws {
			return h.ID, true
		}
	}
	return "", false
}

func normalizeHeading(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// RemoveLeadingTitle drops a level-1 heading that opens the note and
// repeats its title, so themes can show the title once. It reports whether
// it removed one.
func (d *Doc) RemoveLeadingTitle(title string) bool {
	h, ok := d.root.FirstChild().(*ast.Heading)
	if !ok || h.Level != 1 || normalizeHeading(plainText(h, d.src)) != normalizeHeading(title) {
		return false
	}
	d.root.RemoveChild(d.root, h)
	if len(d.Headings) > 0 && d.Headings[0].Level == 1 {
		d.Headings = d.Headings[1:]
	}
	return true
}

// Tags returns the inline tags of the note in order of appearance. Tags in
// comments are not included: comments are stripped before parsing.
func (d *Doc) Tags() []string {
	var out []string
	_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := n.(*Tag); ok && entering {
			out = append(out, t.Name)
		}
		return ast.WalkContinue, nil
	})
	return out
}

// HasBlock reports whether the note defines block id (without '^').
func (d *Doc) HasBlock(id string) bool { return d.blockIDs[id] }

// Section returns the source of the part of the note a subpath points to:
// for "^id" the block, for a heading the heading and everything up to the
// next heading of the same or a higher level.
func (d *Doc) Section(sub string) ([]byte, bool) {
	if id, ok := strings.CutPrefix(sub, "^"); ok {
		var found ast.Node
		_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering && found == nil {
				if v, ok := n.AttributeString("id"); ok {
					if b, ok := v.([]byte); ok && string(b) == "^"+id {
						found = n
						return ast.WalkStop, nil
					}
				}
			}
			return ast.WalkContinue, nil
		})
		if found == nil {
			return nil, false
		}
		start, stop := sourceRange(found)
		if start < 0 {
			return nil, false
		}
		return d.src[lineStart(d.src, start):stop], true
	}
	hid, ok := d.HeadingID(sub)
	if !ok {
		return nil, false
	}
	var start, level = -1, 0
	for c := d.root.FirstChild(); c != nil; c = c.NextSibling() {
		h, ok := c.(*ast.Heading)
		if !ok || h.Lines().Len() == 0 {
			continue
		}
		ls := lineStart(d.src, h.Lines().At(0).Start)
		if start < 0 {
			if v, _ := h.AttributeString("id"); v != nil && string(v.([]byte)) == hid {
				start, level = ls, h.Level
			}
			continue
		}
		if h.Level <= level {
			return d.src[start:ls], true
		}
	}
	if start < 0 {
		return nil, false
	}
	return d.src[start:], true
}

// sourceRange returns the smallest byte range covering n's lines (and its
// descendants' lines).
func sourceRange(n ast.Node) (int, int) {
	start, stop := -1, -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || c.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		lines := c.Lines()
		for i := 0; i < lines.Len(); i++ {
			s := lines.At(i)
			if start < 0 || s.Start < start {
				start = s.Start
			}
			if s.Stop > stop {
				stop = s.Stop
			}
		}
		return ast.WalkContinue, nil
	})
	return start, stop
}

func lineStart(src []byte, i int) int {
	for i > 0 && src[i-1] != '\n' {
		i--
	}
	return i
}
