// Package markdown renders Obsidian-flavored Markdown with goldmark.
//
// Rendering happens in three steps so that links can be resolved against
// the published set: Parse builds the AST (wikilinks, embeds, tags,
// callouts, highlights, block IDs, math), Doc.Resolve asks a Resolver what
// every internal link and embed points to, and Doc.Render writes HTML. A
// link the resolver does not resolve is rendered as plain text and an
// unresolved embed is omitted, so nothing unpublished can be linked.
package markdown

import (
	"github.com/yuin/goldmark/ast"
)

// Kinds of custom nodes.
var (
	KindWikiLink     = ast.NewNodeKind("WikiLink")
	KindTag          = ast.NewNodeKind("Tag")
	KindHighlight    = ast.NewNodeKind("Highlight")
	KindMath         = ast.NewNodeKind("Math")
	KindMathBlock    = ast.NewNodeKind("MathBlock")
	KindCallout      = ast.NewNodeKind("Callout")
	KindCalloutTitle = ast.NewNodeKind("CalloutTitle")
	KindMermaid      = ast.NewNodeKind("Mermaid")
)

// WikiLink is [[target#subpath|alias]] or, with Embed, ![[…]].
type WikiLink struct {
	ast.BaseInline
	Target  string
	Subpath string
	Alias   string
	Embed   bool

	// Set by Resolve.
	Resolved  Target
	EmbedHTML string // rendered content of an embedded note
}

// Kind implements ast.Node.
func (n *WikiLink) Kind() ast.NodeKind { return KindWikiLink }

// Dump implements ast.Node.
func (n *WikiLink) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Target": n.Target, "Subpath": n.Subpath, "Alias": n.Alias}, nil)
}

// Label is what the link shows: the alias, else the target as written.
func (n *WikiLink) Label() string {
	if n.Alias != "" {
		return n.Alias
	}
	switch {
	case n.Target == "" && n.Subpath != "":
		return trimBlockMarker(n.Subpath)
	case n.Subpath != "":
		return n.Target + " > " + trimBlockMarker(n.Subpath)
	}
	return n.Target
}

func trimBlockMarker(s string) string {
	if len(s) > 0 && s[0] == '^' {
		return s[1:]
	}
	return s
}

// Tag is an inline #tag.
type Tag struct {
	ast.BaseInline
	Name string
	URL  string // set by Resolve; "" renders the tag as plain text
}

// Kind implements ast.Node.
func (n *Tag) Kind() ast.NodeKind { return KindTag }

// Dump implements ast.Node.
func (n *Tag) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Name": n.Name}, nil)
}

// Highlight is ==text==.
type Highlight struct{ ast.BaseInline }

// Kind implements ast.Node.
func (n *Highlight) Kind() ast.NodeKind { return KindHighlight }

// Dump implements ast.Node.
func (n *Highlight) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// Math is inline $…$ (or $$…$$ inside a paragraph, with Display).
type Math struct {
	ast.BaseInline
	TeX     string
	Display bool
}

// Kind implements ast.Node.
func (n *Math) Kind() ast.NodeKind { return KindMath }

// Dump implements ast.Node.
func (n *Math) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"TeX": n.TeX}, nil)
}

// MathBlock is a $$ … $$ block.
type MathBlock struct{ ast.BaseBlock }

// Kind implements ast.Node.
func (n *MathBlock) Kind() ast.NodeKind { return KindMathBlock }

// IsRaw implements ast.Node.
func (n *MathBlock) IsRaw() bool { return true }

// Dump implements ast.Node.
func (n *MathBlock) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// Callout is an Obsidian callout: a blockquote starting with [!type]. Its
// first child is a CalloutTitle.
type Callout struct {
	ast.BaseBlock
	CalloutType string // lowercase, e.g. "note", "warning"
	Foldable    bool
	Open        bool
}

// CalloutTitle holds the inline content of a callout's title line. Without
// children the capitalized type is shown.
type CalloutTitle struct{ ast.BaseBlock }

// Kind implements ast.Node.
func (n *CalloutTitle) Kind() ast.NodeKind { return KindCalloutTitle }

// Dump implements ast.Node.
func (n *CalloutTitle) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// Kind implements ast.Node.
func (n *Callout) Kind() ast.NodeKind { return KindCallout }

// Dump implements ast.Node.
func (n *Callout) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Type": n.CalloutType}, nil)
}

// Mermaid is a ```mermaid code block, rendered for client-side drawing.
type Mermaid struct{ ast.BaseBlock }

// Kind implements ast.Node.
func (n *Mermaid) Kind() ast.NodeKind { return KindMermaid }

// IsRaw implements ast.Node.
func (n *Mermaid) IsRaw() bool { return true }

// Dump implements ast.Node.
func (n *Mermaid) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }
