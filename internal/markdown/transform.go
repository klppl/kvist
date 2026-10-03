package markdown

import (
	"bytes"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// transformer rewrites the parsed AST: callouts, Mermaid blocks and block
// IDs. Heading IDs are assigned in Parse because they need per-document
// state.
type transformer struct{}

var (
	calloutRe = regexp.MustCompile(`^\[!([A-Za-z0-9_-]+)\]([+-]?)(?:\s+(.*?))?\s*$`)
	blockIDRe = regexp.MustCompile(`(?:^|\s)\^([A-Za-z0-9-]+)\s*$`)
)

func (transformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	var quotes []*ast.Blockquote
	var fences []*ast.FencedCodeBlock
	var blocks []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Blockquote:
			quotes = append(quotes, n)
		case *ast.FencedCodeBlock:
			fences = append(fences, n)
		case *ast.Paragraph, *ast.TextBlock:
			blocks = append(blocks, n)
		}
		return ast.WalkContinue, nil
	})
	// Innermost first doesn't matter: each conversion only touches its own
	// blockquote and first paragraph.
	for _, q := range quotes {
		toCallout(q, src)
	}
	for _, f := range fences {
		if strings.EqualFold(string(f.Language(src)), "mermaid") {
			m := &Mermaid{}
			m.SetLines(f.Lines())
			f.Parent().ReplaceChild(f.Parent(), f, m)
		}
	}
	for _, b := range blocks {
		extractBlockID(b, src)
	}
}

func toCallout(bq *ast.Blockquote, src []byte) {
	p, ok := bq.FirstChild().(*ast.Paragraph)
	if !ok || p.Lines().Len() == 0 {
		return
	}
	first := p.Lines().At(0)
	m := calloutRe.FindStringSubmatchIndex(strings.TrimRight(string(first.Value(src)), "\r\n"))
	if m == nil {
		return
	}
	line := first.Value(src)
	c := &Callout{CalloutType: strings.ToLower(string(line[m[2]:m[3]])), Foldable: m[5] > m[4], Open: string(line[m[4]:m[5]]) == "+"}
	title := &CalloutTitle{}
	c.AppendChild(c, title)
	titleStart := first.Stop // no title text
	if m[6] >= 0 && m[7] > m[6] {
		titleStart = first.Start + m[6]
	}
	// Move the inline nodes of the first line into the title, dropping the
	// "[!type]+" marker.
	for child := p.FirstChild(); child != nil; {
		next := child.NextSibling()
		t, isText := child.(*ast.Text)
		if isText && t.Segment.Start >= first.Stop {
			break
		}
		endOfLine := isText && (t.SoftLineBreak() || t.HardLineBreak() || t.Segment.Stop >= lineEnd(first, src))
		switch {
		case isText && t.Segment.Stop <= titleStart:
			p.RemoveChild(p, child)
		case isText && t.Segment.Start < titleStart:
			t.Segment = t.Segment.WithStart(titleStart)
			fallthrough
		default:
			if isText {
				t.SetSoftLineBreak(false)
				t.SetHardLineBreak(false)
			}
			title.AppendChild(title, child)
		}
		if endOfLine {
			break
		}
		child = next
	}
	lines := text.NewSegments()
	for i := 1; i < p.Lines().Len(); i++ {
		lines.Append(p.Lines().At(i))
	}
	p.SetLines(lines)
	if p.ChildCount() == 0 {
		bq.RemoveChild(bq, p)
	}
	for ch := bq.FirstChild(); ch != nil; {
		next := ch.NextSibling()
		c.AppendChild(c, ch)
		ch = next
	}
	bq.Parent().ReplaceChild(bq.Parent(), bq, c)
}

func lineEnd(seg text.Segment, src []byte) int {
	stop := seg.Stop
	for stop > seg.Start && (src[stop-1] == '\n' || src[stop-1] == '\r') {
		stop--
	}
	return stop
}

func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// extractBlockID handles a trailing "^block-id": it is removed from the text
// and set as the id of the paragraph (or of the list item it belongs to).
func extractBlockID(b ast.Node, src []byte) {
	last := b.LastChild()
	t, ok := last.(*ast.Text)
	if !ok {
		return
	}
	val := t.Segment.Value(src)
	loc := blockIDRe.FindSubmatchIndex(val)
	if loc == nil {
		return
	}
	id := string(val[loc[2]:loc[3]])
	newStop := t.Segment.Start + loc[0]
	for newStop > t.Segment.Start && (src[newStop-1] == ' ' || src[newStop-1] == '\t') {
		newStop--
	}
	if newStop <= t.Segment.Start {
		b.RemoveChild(b, t)
		if prev, ok := b.LastChild().(*ast.Text); ok {
			prev.SetSoftLineBreak(false)
			prev.SetHardLineBreak(false)
		}
	} else {
		t.Segment = t.Segment.WithStop(newStop)
	}
	target := b
	if li, ok := b.Parent().(*ast.ListItem); ok && li.FirstChild() == b {
		target = li
	}
	target.SetAttributeString("id", []byte("^"+id))
}

// plainText returns the text content of n.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	writePlain(&b, n, src)
	return strings.Join(strings.Fields(b.String()), " ")
}

func writePlain(b *strings.Builder, n ast.Node, src []byte) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(src))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(c.Value)
		case *WikiLink:
			if !c.Embed {
				b.WriteString(c.Label())
			}
		case *Tag:
			b.WriteString("#" + c.Name)
		case *Math:
			b.WriteString(c.TeX)
		case *ast.Image:
			// alt text is not content
		case *ast.RawHTML, *ast.HTMLBlock:
		case *ast.FencedCodeBlock, *ast.CodeBlock, *MathBlock, *Mermaid:
			b.WriteByte(' ')
			b.Write(linesValue(c, src))
			b.WriteByte(' ')
		default:
			writePlain(b, c, src)
			if c.Type() == ast.TypeBlock {
				b.WriteByte(' ')
			}
		}
	}
}

func linesValue(n ast.Node, src []byte) []byte {
	var buf bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(src))
	}
	return buf.Bytes()
}
