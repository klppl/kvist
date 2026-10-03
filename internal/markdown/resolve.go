package markdown

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// Ref is an internal link or embed as written in a note.
type Ref struct {
	Target   string // as written; "" for a link within the same note
	Subpath  string // heading or "^block", without '#'
	Embed    bool
	Markdown bool // [text](target) rather than [[target]]
}

// TargetKind says what a reference resolved to.
type TargetKind int

// Target kinds.
const (
	Unresolved TargetKind = iota // missing or not published: rendered as text
	NoteTarget
	AssetTarget
)

// Media types of assets, for embeds.
const (
	MediaImage = "image"
	MediaAudio = "audio"
	MediaVideo = "video"
	MediaPDF   = "pdf"
	MediaOther = "other"
)

// Target is a resolved reference.
type Target struct {
	Kind  TargetKind
	URL   string // including the #fragment for headings and blocks
	Path  string // vault path of the target
	Title string // note title, for embed headers
	Media string // for assets
}

// Resolver decides what references point to. Implementations must only
// return targets that are published.
type Resolver interface {
	// Resolve resolves one reference.
	Resolve(ref Ref) Target
	// Embed returns the rendered HTML of an embedded note (or of a section
	// of it), or false to omit the embed.
	Embed(target Target, ref Ref) (string, bool)
	// TagURL returns the URL of a tag page, or "" to render the tag as
	// plain text (for control tags such as #public).
	TagURL(name string) string
}

// ResolvedRef is a reference after resolution.
type ResolvedRef struct {
	Ref
	Target  Target
	Context string // plain text of the surrounding block, for backlinks
}

// Resolve resolves every reference and tag in the document. It must be
// called before Render.
func (d *Doc) Resolve(r Resolver) {
	for _, rn := range d.refs {
		t := r.Resolve(rn.Ref)
		switch n := rn.node.(type) {
		case *WikiLink:
			if n.Embed && t.Kind == NoteTarget {
				html, ok := r.Embed(t, rn.Ref)
				if !ok {
					t = Target{}
				}
				n.EmbedHTML = html
			}
			n.Resolved = t
		case *ast.Image:
			if t.Kind != AssetTarget {
				t = Target{} // images can only show assets
			}
		}
		rn.Target = t
		rn.Context = blockContext(rn.node, d.src)
	}
	_ = ast.Walk(d.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if tag, ok := n.(*Tag); ok && entering {
			tag.URL = r.TagURL(tag.Name)
		}
		return ast.WalkContinue, nil
	})
}

// Refs returns the resolved references in document order.
func (d *Doc) Refs() []ResolvedRef {
	out := make([]ResolvedRef, len(d.refs))
	for i, rn := range d.refs {
		out[i] = ResolvedRef{Ref: rn.Ref, Target: rn.Target, Context: rn.Context}
	}
	return out
}

// maxContext bounds backlink context snippets.
const maxContext = 280

func blockContext(n ast.Node, src []byte) string {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Type() == ast.TypeBlock {
			t := plainText(p, src)
			if len(t) > maxContext {
				cut := strings.LastIndexByte(t[:maxContext], ' ')
				if cut < maxContext/2 {
					cut = maxContext
				}
				t = t[:cut] + "…"
			}
			return t
		}
	}
	return ""
}
