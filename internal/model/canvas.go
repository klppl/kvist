package model

import (
	"fmt"
	"html"
	"math"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/klppl/kvist/internal/markdown"
	"github.com/klppl/kvist/internal/publish"
	"github.com/klppl/kvist/internal/slug"
	"github.com/klppl/kvist/internal/vault"
)

// WarnCanvas is a canvas file that could not be read.
const WarnCanvas = "canvas"

// Canvases (.canvas files) are published as pages of their own. A canvas
// has no frontmatter or tags, so the rules are the folder's and the
// attachments': it is published when it lies in an always-public folder,
// or when a published note or canvas links to or embeds it, and never from
// an excluded folder. Its cards go through the same resolver as a note's
// links and embeds, so a card showing an unpublished note is left out, as
// an embed of it would be. The canvas file itself is never output: it names
// the vault paths of every card.

// pickCanvases decides which canvases are published and adds them to the
// published notes.
func (b *builder) pickCanvases() {
	var queue []string
	queued := map[string]bool{}
	add := func(p string) {
		if _, ok := b.canvases[p]; ok && !queued[p] {
			queued[p] = true
			queue = append(queue, p)
		}
	}
	var seeds []string
	for p := range b.canvases {
		if publish.CanvasInPublicFolder(b.rules, p) {
			seeds = append(seeds, p)
		}
	}
	for p, ns := range b.notes {
		for _, l := range ns.meta.Links {
			if t := b.lookup(p, markdown.Ref{Target: l.Target, Markdown: l.Markdown}); vault.IsCanvas(t) {
				seeds = append(seeds, t)
			}
		}
	}
	sort.Strings(seeds)
	for _, p := range seeds {
		add(p)
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		f := b.canvases[p]
		src, err := b.read(f)
		if err != nil {
			b.warn(WarnCanvas, p, "canvas could not be read; left out")
			continue
		}
		c, err := vault.ParseCanvas(src)
		if err != nil {
			b.warn(WarnCanvas, p, "canvas is not valid JSON; left out")
			continue
		}
		b.notes[p] = &noteState{file: f, src: src, meta: &vault.Meta{Frontmatter: map[string]any{}}, doc: markdown.Parse(nil), canvas: c}
		for _, l := range c.Links() {
			if t := b.lookup(p, markdown.Ref{Target: l.Target, Markdown: l.Markdown}); vault.IsCanvas(t) {
				add(t)
			}
		}
	}
}

// makeCanvasNote is makeNotes for a canvas: it has no properties, so its
// title is its file name and its dates are the file's.
func (b *builder) makeCanvasNote(p string, ns *noteState) *Note {
	n := &Note{Path: p, Canvas: true}
	n.Title = strings.TrimSuffix(path.Base(p), path.Ext(p))
	// "Board.canvas" → /board-canvas/, so a note named Board can sit next to it.
	u := slug.Path(b.sitePath(p))
	if u == "" {
		u = slug.Make(fmt.Sprintf("canvas-%x", p))
	}
	n.URL = "/" + u + "/"
	n.Slug = strings.Trim(n.URL, "/")
	n.ID = n.Slug
	n.Updated = ns.file.MTime.UTC()
	n.Created = n.Updated
	return n
}

// canvasRender is a rendered canvas.
type canvasRender struct {
	html  string
	text  []string // plain text of the cards and labels, for search
	refs  []markdown.ResolvedRef
	tags  []string
	first string // the first paragraph of the first text card
}

type canvasCard struct {
	node  vault.CanvasNode
	inner string
}

// boardPad is the room around the cards, for arrows that curve outwards.
const boardPad = 40

var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// colorAttr turns a canvas color (a preset "1"–"6" or a hex color) into a
// class and an inline style; anything else is no color.
func colorAttr(c string) (class, style string) {
	switch {
	case len(c) == 1 && c[0] >= '1' && c[0] <= '6':
		return " canvas-color-" + c, ""
	case hexColor.MatchString(c):
		return " canvas-color", "--canvas-color:" + c + ";"
	}
	return "", ""
}

// renderCanvas renders ns's canvas as a board: absolutely placed cards and
// an SVG layer with the arrows. r resolves the cards' links and embeds.
func (b *builder) renderCanvas(ns *noteState, r *noteResolver) canvasRender {
	var out canvasRender
	var groups, cards []canvasCard
	boxes := map[string]vault.CanvasNode{}
	for _, nd := range ns.canvas.Nodes {
		if !(nd.Width > 0 && nd.Height > 0) || !onBoard(nd) {
			continue
		}
		var inner string
		switch nd.Type {
		case "group":
			if nd.Label != "" {
				inner = `<div class="canvas-group-label">` + html.EscapeString(nd.Label) + `</div>`
				out.text = append(out.text, nd.Label)
			}
			groups = append(groups, canvasCard{nd, inner})
			boxes[nd.ID] = nd
			continue
		case "text":
			doc := markdown.Parse([]byte(nd.Text))
			doc.StrictLineBreaks = b.strict
			doc.Resolve(r)
			h, err := doc.Render()
			if err != nil {
				continue
			}
			inner = `<div class="canvas-node-content">` + h + `</div>`
			out.refs = append(out.refs, doc.Refs()...)
			out.tags = append(out.tags, doc.Tags()...)
			out.text = append(out.text, doc.PlainText())
			if out.first == "" {
				out.first = doc.FirstParagraph()
			}
			r.root.Features.Math = r.root.Features.Math || doc.Features.Math
			r.root.Features.Mermaid = r.root.Features.Mermaid || doc.Features.Mermaid
			r.root.Features.Code = r.root.Features.Code || doc.Features.Code
		case "file":
			var ok bool
			inner, ok = b.canvasFile(nd, r, &out)
			if !ok {
				continue
			}
		case "link":
			u, err := url.Parse(strings.TrimSpace(nd.URL))
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				continue
			}
			label := strings.TrimSuffix(u.Host+u.EscapedPath(), "/")
			inner = fmt.Sprintf(`<div class="canvas-node-content"><a class="external-link" href="%s" rel="noopener">%s</a></div>`,
				html.EscapeString(u.String()), html.EscapeString(label))
		default:
			continue
		}
		cards = append(cards, canvasCard{nd, inner})
		boxes[nd.ID] = nd
	}

	// The board spans the cards, with room around them.
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, nd := range boxes {
		minX, minY = math.Min(minX, nd.X), math.Min(minY, nd.Y)
		maxX, maxY = math.Max(maxX, nd.X+nd.Width), math.Max(maxY, nd.Y+nd.Height)
	}
	if len(boxes) == 0 {
		minX, minY, maxX, maxY = 0, 0, 0, 0
	}
	ox, oy := minX-boardPad, minY-boardPad
	w, h := px(maxX-minX+2*boardPad), px(maxY-minY+2*boardPad)

	var sb strings.Builder
	fmt.Fprintf(&sb, `<div class="canvas"><div class="canvas-board" style="width:%dpx;height:%dpx">`, w, h)
	writeCard := func(c canvasCard) {
		class, style := colorAttr(c.node.Color)
		fmt.Fprintf(&sb, `<div class="canvas-node canvas-%s%s" style="left:%dpx;top:%dpx;width:%dpx;height:%dpx;%s">%s</div>`,
			c.node.Type, class, px(c.node.X-ox), px(c.node.Y-oy), px(c.node.Width), px(c.node.Height), style, c.inner)
	}
	for _, c := range groups {
		writeCard(c)
	}
	var labels strings.Builder
	fmt.Fprintf(&sb, `<svg class="canvas-edges" width="%d" height="%d" viewBox="0 0 %d %d" aria-hidden="true">`, w, h, w, h)
	for _, e := range ns.canvas.Edges {
		from, ok1 := boxes[e.FromNode]
		to, ok2 := boxes[e.ToNode]
		if !ok1 || !ok2 {
			continue // an end was left out, so is the arrow (and its label)
		}
		x1, y1, nx1, ny1 := anchor(from, to, e.FromSide)
		x2, y2, nx2, ny2 := anchor(to, from, e.ToSide)
		x1, y1, x2, y2 = x1-ox, y1-oy, x2-ox, y2-oy
		off := math.Min(150, math.Max(30, math.Hypot(x2-x1, y2-y1)/2))
		c1x, c1y := x1+nx1*off, y1+ny1*off
		c2x, c2y := x2+nx2*off, y2+ny2*off
		class, style := colorAttr(e.Color)
		if style != "" {
			style = ` style="` + style + `"`
		}
		fmt.Fprintf(&sb, `<g class="canvas-edge%s"%s><path d="M%s %s C%s %s %s %s %s %s"/>`, class, style,
			num(x1), num(y1), num(c1x), num(c1y), num(c2x), num(c2y), num(x2), num(y2))
		if e.ToEnd != "none" {
			sb.WriteString(arrowHead(x2, y2, -nx2, -ny2))
		}
		if e.FromEnd == "arrow" {
			sb.WriteString(arrowHead(x1, y1, -nx1, -ny1))
		}
		sb.WriteString(`</g>`)
		if e.Label != "" {
			// The middle of the curve.
			mx := .125*x1 + .375*c1x + .375*c2x + .125*x2
			my := .125*y1 + .375*c1y + .375*c2y + .125*y2
			fmt.Fprintf(&labels, `<div class="canvas-edge-label" style="left:%dpx;top:%dpx">%s</div>`, px(mx), px(my), html.EscapeString(e.Label))
			out.text = append(out.text, e.Label)
		}
	}
	sb.WriteString(`</svg>`)
	for _, c := range cards {
		writeCard(c)
	}
	sb.WriteString(labels.String())
	sb.WriteString(`</div></div>`)
	out.html = sb.String()
	return out
}

// canvasFile renders a file card: a published note shown as an embed of it,
// a published canvas as a link to it, an attachment as its embed. Anything
// else is left out.
func (b *builder) canvasFile(nd vault.CanvasNode, r *noteResolver, out *canvasRender) (string, bool) {
	ref := markdown.Ref{Target: nd.File, Subpath: strings.TrimPrefix(nd.Subpath, "#"), Embed: true}
	t := r.Resolve(ref)
	switch t.Kind {
	case markdown.NoteTarget:
		title := fmt.Sprintf(`<div class="canvas-file-title"><a class="internal-link" href="%s">%s</a></div>`, html.EscapeString(t.URL), html.EscapeString(t.Title))
		out.refs = append(out.refs, markdown.ResolvedRef{Ref: ref, Target: t})
		if b.notes[t.Path].canvas != nil {
			return title, true
		}
		body, ok := r.Embed(t, ref)
		if !ok {
			return "", false
		}
		return title + `<div class="canvas-node-content">` + body + `</div>`, true
	case markdown.AssetTarget:
		return `<div class="canvas-node-content">` + markdown.AssetHTML(t, "", path.Base(nd.File)) + `</div>`, true
	}
	return "", false
}

// anchor is where an arrow meets card n: the middle of the given side, or
// of the side facing the other card. It returns the point and the side's
// outward direction.
func anchor(n, other vault.CanvasNode, side string) (x, y, nx, ny float64) {
	if side == "" {
		dx := (other.X + other.Width/2) - (n.X + n.Width/2)
		dy := (other.Y + other.Height/2) - (n.Y + n.Height/2)
		switch {
		case math.Abs(dx) >= math.Abs(dy) && dx >= 0:
			side = "right"
		case math.Abs(dx) >= math.Abs(dy):
			side = "left"
		case dy >= 0:
			side = "bottom"
		default:
			side = "top"
		}
	}
	switch side {
	case "top":
		return n.X + n.Width/2, n.Y, 0, -1
	case "bottom":
		return n.X + n.Width/2, n.Y + n.Height, 0, 1
	case "left":
		return n.X, n.Y + n.Height/2, -1, 0
	}
	return n.X + n.Width, n.Y + n.Height/2, 1, 0
}

// arrowHead is a triangle with its tip at (x, y), pointing along (dx, dy).
func arrowHead(x, y, dx, dy float64) string {
	const length, half = 12, 6
	bx, by := x-dx*length, y-dy*length
	return fmt.Sprintf(`<path class="canvas-arrow" d="M%s %s L%s %s L%s %s Z"/>`,
		num(x), num(y), num(bx-dy*half), num(by+dx*half), num(bx+dy*half), num(by-dx*half))
}

// maxCoord bounds card positions and sizes, so a broken canvas can't ask
// for a board millions of pixels wide.
const maxCoord = 1e6

func onBoard(nd vault.CanvasNode) bool {
	for _, v := range []float64{nd.X, nd.Y, nd.Width, nd.Height} {
		if math.Abs(v) > maxCoord {
			return false
		}
	}
	return true
}

func px(v float64) int { return int(math.Round(v)) }

func num(v float64) string { return fmt.Sprint(px(v)) }
