package markdown

import (
	"bytes"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Render writes the document as HTML. Resolve must have been called.
func (d *Doc) Render() (string, error) {
	nr := &nodeRenderer{doc: d}
	opts := []gmhtml.Option{gmhtml.WithUnsafe(), gmhtml.WithHardWraps()}
	r := renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(gmhtml.NewRenderer(opts...), 1000),
		util.Prioritized(extension.NewTableHTMLRenderer(), 500),
		util.Prioritized(extension.NewStrikethroughHTMLRenderer(opts...), 500),
		util.Prioritized(extension.NewTaskCheckBoxHTMLRenderer(opts...), 500),
		util.Prioritized(extension.NewFootnoteHTMLRenderer(), 500),
		util.Prioritized(nr, 100),
	))
	var buf bytes.Buffer
	if err := r.Render(&buf, d.src, d.root); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type nodeRenderer struct {
	doc *Doc
}

func (r *nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindWikiLink, r.wikiLink)
	reg.Register(KindTag, r.tag)
	reg.Register(KindHighlight, r.highlight)
	reg.Register(KindMath, r.math)
	reg.Register(KindMathBlock, r.mathBlock)
	reg.Register(KindCallout, r.callout)
	reg.Register(KindCalloutTitle, r.calloutTitle)
	reg.Register(KindMermaid, r.mermaid)
	reg.Register(ast.KindLink, r.link)
	reg.Register(ast.KindImage, r.image)
	reg.Register(ast.KindParagraph, r.paragraph)
	reg.Register(ast.KindFencedCodeBlock, r.codeBlock)
	reg.Register(ast.KindCodeBlock, r.codeBlock)
}

func esc(s string) string { return html.EscapeString(s) }

func (r *nodeRenderer) target(n ast.Node) (Target, bool) {
	for _, rn := range r.doc.refs {
		if rn.node == n {
			return rn.Target, true
		}
	}
	return Target{}, false
}

func (r *nodeRenderer) wikiLink(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*WikiLink)
	t := n.Resolved
	if !n.Embed {
		writeLink(w, t, esc(n.Label()))
		return ast.WalkSkipChildren, nil
	}
	switch t.Kind {
	case NoteTarget:
		fmt.Fprintf(w, `<div class="embed"><div class="embed-title"><a class="internal-link" href="%s">%s</a></div><div class="embed-content">%s</div></div>`,
			esc(t.URL), esc(t.Title), n.EmbedHTML)
	case AssetTarget:
		writeAsset(w, t, n.Alias, n.Target)
	}
	// Unresolved embeds are omitted entirely.
	return ast.WalkSkipChildren, nil
}

func writeLink(w util.BufWriter, t Target, innerHTML string) {
	if t.Kind == Unresolved {
		fmt.Fprintf(w, `<span class="link-unpublished">%s</span>`, innerHTML)
		return
	}
	fmt.Fprintf(w, `<a class="internal-link" href="%s">%s</a>`, esc(t.URL), innerHTML)
}

// writeAsset embeds an attachment. For images the alias may be a size
// ("300" or "300x200"), else it is the alt text.
func writeAsset(w util.BufWriter, t Target, alias, name string) {
	switch t.Media {
	case MediaImage:
		alt := alias
		var size string
		if wd, ht, ok := parseSize(alias); ok {
			alt = ""
			size = fmt.Sprintf(` width="%d"`, wd)
			if ht > 0 {
				size += fmt.Sprintf(` height="%d"`, ht)
			}
		}
		fmt.Fprintf(w, `<img src="%s" alt="%s"%s loading="lazy">`, esc(t.URL), esc(alt), size)
	case MediaAudio:
		fmt.Fprintf(w, `<audio controls preload="metadata" src="%s"></audio>`, esc(t.URL))
	case MediaVideo:
		fmt.Fprintf(w, `<video controls preload="metadata" src="%s"></video>`, esc(t.URL))
	case MediaPDF:
		fmt.Fprintf(w, `<iframe class="embed-pdf" src="%s" title="%s" loading="lazy"></iframe>`, esc(t.URL), esc(name))
	default:
		label := alias
		if label == "" {
			label = name
		}
		fmt.Fprintf(w, `<a class="internal-link" href="%s">%s</a>`, esc(t.URL), esc(label))
	}
}

func parseSize(s string) (int, int, bool) {
	wStr, hStr, hasH := strings.Cut(strings.TrimSpace(s), "x")
	wd, err := strconv.Atoi(wStr)
	if err != nil || wd <= 0 {
		return 0, 0, false
	}
	if !hasH {
		return wd, 0, true
	}
	ht, err := strconv.Atoi(hStr)
	if err != nil || ht <= 0 {
		return 0, 0, false
	}
	return wd, ht, true
}

func (r *nodeRenderer) tag(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*Tag)
		if n.URL == "" {
			fmt.Fprintf(w, `<span class="tag tag-hidden">#%s</span>`, esc(n.Name))
		} else {
			fmt.Fprintf(w, `<a class="tag" href="%s">#%s</a>`, esc(n.URL), esc(n.Name))
		}
	}
	return ast.WalkSkipChildren, nil
}

func (r *nodeRenderer) highlight(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.WriteString("<mark>")
	} else {
		w.WriteString("</mark>")
	}
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) math(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*Math)
		if n.Display {
			fmt.Fprintf(w, `<span class="math math-display">\[%s\]</span>`, esc(n.TeX))
		} else {
			fmt.Fprintf(w, `<span class="math math-inline">\(%s\)</span>`, esc(n.TeX))
		}
	}
	return ast.WalkSkipChildren, nil
}

func (r *nodeRenderer) mathBlock(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		fmt.Fprintf(w, "<div class=\"math math-display\">\\[%s\\]</div>\n", esc(string(linesValue(node, src))))
	}
	return ast.WalkSkipChildren, nil
}

func (r *nodeRenderer) mermaid(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		fmt.Fprintf(w, "<pre class=\"mermaid\">%s</pre>\n", esc(string(linesValue(node, src))))
	}
	return ast.WalkSkipChildren, nil
}

func (r *nodeRenderer) callout(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Callout)
	if entering {
		if n.Foldable {
			open := ""
			if n.Open {
				open = " open"
			}
			fmt.Fprintf(w, `<details class="callout" data-callout="%s"%s>`, esc(n.CalloutType), open)
		} else {
			fmt.Fprintf(w, `<div class="callout" data-callout="%s">`, esc(n.CalloutType))
		}
		return ast.WalkContinue, nil
	}
	if n.Foldable {
		w.WriteString("</div></details>\n")
	} else {
		w.WriteString("</div></div>\n")
	}
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) calloutTitle(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	foldable := node.Parent().(*Callout).Foldable
	tag := "div"
	if foldable {
		tag = "summary"
	}
	if entering {
		fmt.Fprintf(w, `<%s class="callout-title">`, tag)
		if node.ChildCount() == 0 {
			w.WriteString(esc(capitalize(node.Parent().(*Callout).CalloutType)))
		}
		return ast.WalkContinue, nil
	}
	fmt.Fprintf(w, "</%s><div class=\"callout-content\">\n", tag)
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) link(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	t, internal := r.target(n)
	if internal {
		if entering {
			if t.Kind == Unresolved {
				w.WriteString(`<span class="link-unpublished">`)
			} else {
				fmt.Fprintf(w, `<a class="internal-link" href="%s">`, esc(t.URL))
			}
		} else if t.Kind == Unresolved {
			w.WriteString("</span>")
		} else {
			w.WriteString("</a>")
		}
		return ast.WalkContinue, nil
	}
	if entering {
		dest := string(n.Destination)
		if gmhtml.IsDangerousURL(n.Destination) {
			dest = "#"
		}
		fmt.Fprintf(w, `<a class="external-link" href="%s"`, esc(dest))
		if len(n.Title) > 0 {
			fmt.Fprintf(w, ` title="%s"`, esc(string(n.Title)))
		}
		w.WriteString(` rel="noopener">`)
	} else {
		w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) image(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	alt := plainText(n, src)
	if t, internal := r.target(n); internal {
		if t.Kind == AssetTarget {
			writeAsset(w, t, alt, alt)
		}
		return ast.WalkSkipChildren, nil
	}
	dest := string(n.Destination)
	if gmhtml.IsDangerousURL(n.Destination) {
		return ast.WalkSkipChildren, nil
	}
	fmt.Fprintf(w, `<img src="%s" alt="%s" loading="lazy">`, esc(dest), esc(alt))
	return ast.WalkSkipChildren, nil
}

// paragraph drops the <p> around a lone note embed (a block in a <p> is
// invalid HTML) and around a lone omitted embed.
func (r *nodeRenderer) paragraph(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if wl, ok := node.FirstChild().(*WikiLink); ok && node.ChildCount() == 1 && wl.Embed &&
		(wl.Resolved.Kind == NoteTarget || wl.Resolved.Kind == Unresolved) {
		return ast.WalkContinue, nil
	}
	if entering {
		if node.Attributes() != nil {
			w.WriteString("<p")
			gmhtml.RenderAttributes(w, node, gmhtml.ParagraphAttributeFilter)
			w.WriteByte('>')
		} else {
			w.WriteString("<p>")
		}
	} else {
		w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

var codeFormatter = chromahtml.New(chromahtml.WithClasses(true), chromahtml.TabWidth(4))

func (r *nodeRenderer) codeBlock(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	lang := ""
	if f, ok := node.(*ast.FencedCodeBlock); ok {
		lang = strings.ToLower(string(f.Language(src)))
	}
	code := string(linesValue(node, src))
	var lexer chroma.Lexer
	if lang != "" {
		lexer = lexers.Get(lang)
	}
	if lexer == nil {
		fmt.Fprintf(w, "<pre><code>%s</code></pre>\n", esc(code))
		return ast.WalkSkipChildren, nil
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		fmt.Fprintf(w, "<pre><code>%s</code></pre>\n", esc(code))
		return ast.WalkSkipChildren, nil
	}
	fmt.Fprintf(w, `<div class="code-block" data-lang="%s">`, esc(lang))
	if err := codeFormatter.Format(w, styles.Fallback, it); err != nil {
		return ast.WalkStop, err
	}
	w.WriteString("</div>\n")
	return ast.WalkSkipChildren, nil
}

// HighlightCSS returns the chroma stylesheet for a style name, for themes.
func HighlightCSS(style string) (string, error) {
	s := styles.Get(style)
	var buf bytes.Buffer
	if err := codeFormatter.WriteCSS(&buf, s); err != nil {
		return "", err
	}
	return buf.String(), nil
}
