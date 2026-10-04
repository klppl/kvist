package markdown

import (
	"strings"
	"testing"
)

// fakeResolver publishes "Public" (with a heading "Intro" and block "b1")
// and "pic.png"; everything else is unresolved.
type fakeResolver struct{ embeds int }

func (f *fakeResolver) Resolve(ref Ref) Target {
	switch strings.ToLower(ref.Target) {
	case "public", "public.md":
		u := "/public/"
		if ref.Subpath != "" {
			u += "#" + strings.ToLower(ref.Subpath)
		}
		return Target{Kind: NoteTarget, URL: u, Path: "Public.md", Title: "Public"}
	case "":
		return Target{Kind: NoteTarget, URL: "#" + strings.ToLower(ref.Subpath), Path: "Self.md"}
	case "pic.png", "img/pic.png":
		return Target{Kind: AssetTarget, URL: "/_assets/abc/pic.png", Path: "img/pic.png", Media: MediaImage}
	case "doc.pdf":
		return Target{Kind: AssetTarget, URL: "/_assets/def/doc.pdf", Path: "doc.pdf", Media: MediaPDF}
	}
	return Target{}
}

func (f *fakeResolver) Embed(t Target, ref Ref) (string, bool) {
	f.embeds++
	return "<p>embedded</p>", true
}

func (f *fakeResolver) TagURL(name string) string {
	if strings.EqualFold(name, "public") {
		return ""
	}
	return "/tags/" + strings.ToLower(name) + "/"
}

func render(t *testing.T, src string) (string, *Doc) {
	t.Helper()
	d := Parse([]byte(src))
	d.Resolve(&fakeResolver{})
	out, err := d.Render()
	if err != nil {
		t.Fatal(err)
	}
	return out, d
}

func contains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q\n---\n%s", w, out)
		}
	}
}

func lacks(t *testing.T, out string, nots ...string) {
	t.Helper()
	for _, n := range nots {
		if strings.Contains(out, n) {
			t.Errorf("output contains %q\n---\n%s", n, out)
		}
	}
}

func TestWikiLinks(t *testing.T) {
	out, d := render(t, "See [[Public]], [[Public#Intro|the intro]], [[Secret Note]] and [[Secret|alias]] and [[#Local]].\n")
	contains(t, out,
		`<a class="internal-link" href="/public/">Public</a>`,
		`<a class="internal-link" href="/public/#intro">the intro</a>`,
		`<span class="link-unpublished">Secret Note</span>`,
		`<span class="link-unpublished">alias</span>`,
		`<a class="internal-link" href="#local">Local</a>`,
	)
	if refs := d.Refs(); len(refs) != 5 || refs[2].Target.Kind != Unresolved {
		t.Errorf("refs = %+v", refs)
	}
}

func TestEmbeds(t *testing.T) {
	out, _ := render(t, "![[Public]]\n\n![[Secret]]\n\nText ![[pic.png|300]] and ![[pic.png|A map]] ![[doc.pdf]]\n")
	contains(t, out,
		`<div class="embed"><div class="embed-title"><a class="internal-link" href="/public/">Public</a></div><div class="embed-content"><p>embedded</p></div></div>`,
		`<img src="/_assets/abc/pic.png" alt="" width="300" loading="lazy">`,
		`<img src="/_assets/abc/pic.png" alt="A map" loading="lazy">`,
		`<iframe class="embed-pdf" src="/_assets/def/doc.pdf"`,
	)
	lacks(t, out, "Secret", "<p><div")
}

func TestMarkdownLinksAndImages(t *testing.T) {
	out, _ := render(t, "[a](Public.md) [b](Secret.md) [c](https://example.com) ![x](img/pic.png) ![y](private.png) [d](javascript:alert(1))\n")
	contains(t, out,
		`<a class="internal-link" href="/public/">a</a>`,
		`<span class="link-unpublished">b</span>`,
		`<a class="external-link" href="https://example.com" rel="noopener">c</a>`,
		`<img src="/_assets/abc/pic.png" alt="x" loading="lazy">`,
	)
	lacks(t, out, "private.png", "javascript:")
}

func TestCommentsAreStripped(t *testing.T) {
	out, d := render(t, "Visible %%hidden [[Public]]%% text\n\n%%\nblock SECRET\n%%\n\n<!-- html SECRET -->\n\n`%%kept%%`\n")
	lacks(t, out, "hidden", "SECRET", "<!--")
	contains(t, out, "<code>%%kept%%</code>")
	if len(d.Refs()) != 0 {
		t.Errorf("link inside a comment was kept: %+v", d.Refs())
	}
}

func TestTagsAndHighlight(t *testing.T) {
	out, _ := render(t, "A #idea/nested and #public and ==marked== text, not#tag, `#code`.\n")
	contains(t, out,
		`<a class="tag" href="/tags/idea/nested/">#idea/nested</a>`,
		`<span class="tag tag-hidden">#public</span>`,
		`<mark>marked</mark>`,
		"not#tag",
		"<code>#code</code>",
	)
}

func TestCallouts(t *testing.T) {
	out, _ := render(t, "> [!warning] Be careful\n> Body **text**\n\n> [!quote] See [[Public]] *now*\n\n> [!tip]- Folded\n> hidden\n\n> [!note]\n> > [!info] Nested\n> > inner\n\n> plain quote\n")
	contains(t, out,
		`<div class="callout" data-callout="warning"><div class="callout-title">Be careful</div><div class="callout-content">`,
		`Body <strong>text</strong>`,
		`<details class="callout" data-callout="tip"><summary class="callout-title">Folded</summary>`,
		`<div class="callout" data-callout="note"><div class="callout-title">Note</div>`,
		`data-callout="info"`,
		"<blockquote>\n<p>plain quote</p>",
		`<div class="callout-title">See <a class="internal-link" href="/public/">Public</a> <em>now</em></div>`,
	)
	lacks(t, out, "[!warning]", "[!tip]")
}

func TestHeadingsBlocksAndSections(t *testing.T) {
	src := "# Title\n\nIntro para ^first\n\n## Part A\n\nA text\n\n### Sub\n\nsub text\n\n## Part A\n\nsecond\n\n- item one ^li\n- item two\n"
	out, d := render(t, src)
	contains(t, out, `<h1 id="title">Title</h1>`, `<h2 id="part-a">Part A</h2>`, `<h2 id="part-a-1">Part A</h2>`,
		`<p id="^first">Intro para</p>`, `<li id="^li">item one</li>`)
	lacks(t, out, "^first<", "^li<")
	if d.Title() != "Title" || d.FirstParagraph() != "Intro para" {
		t.Errorf("title %q, first paragraph %q", d.Title(), d.FirstParagraph())
	}
	if id, ok := d.HeadingID("part  a"); !ok || id != "part-a" {
		t.Errorf("HeadingID = %q %v", id, ok)
	}
	sec, ok := d.Section("Part A")
	if !ok || string(sec) != "## Part A\n\nA text\n\n### Sub\n\nsub text\n\n" {
		t.Errorf("section = %q", sec)
	}
	if blk, ok := d.Section("^first"); !ok || string(blk) != "Intro para ^first" {
		t.Errorf("block = %q", blk)
	}
	if !d.HasBlock("li") {
		t.Error("list item block id not found")
	}
}

func TestMathMermaidCode(t *testing.T) {
	out, d := render(t, "Inline $a_b$ and $5 and $6 dollars.\n\n$$\nx^2\n$$\n\n```mermaid\ngraph TD; A-->B\n```\n\n```go\nfunc main() {}\n```\n")
	contains(t, out,
		`<span class="math math-inline">\(a_b\)</span>`,
		"$5 and $6 dollars",
		"<div class=\"math math-display\">\\[x^2\n\\]</div>",
		`<pre class="mermaid">graph TD; A--&gt;B`,
		`<div class="code-block" data-lang="go"><pre class="chroma">`,
	)
	if !d.Features.Math || !d.Features.Mermaid || !d.Features.Code {
		t.Errorf("features = %+v", d.Features)
	}
}

func TestGFM(t *testing.T) {
	out, _ := render(t, "| a | b |\n|---|---|\n| [[Public\\|p]] | 2 |\n\n- [ ] todo\n- [x] done\n\n~~gone~~ and footnote[^1]\n\n[^1]: note\n")
	contains(t, out, "<table>", `<a class="internal-link" href="/public/">p</a>`, `type="checkbox"`, "<del>gone</del>", `class="footnote-ref"`)
}

func TestPlainTextLeavesOutEmbeds(t *testing.T) {
	_, d := render(t, "Hello [[Secret|world]] ![[Secret]] `code`\n")
	if got := d.PlainText(); got != "Hello world code" {
		t.Errorf("plain text = %q", got)
	}
}

func TestVideoEmbeds(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":      "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?t=1m30s":             "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ?start=90",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ&t=42":   "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ?start=42",
		"https://youtube.com/shorts/dQw4w9WgXcQ":           "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ",
		"https://vimeo.com/76979871":                       "https://player.vimeo.com/video/76979871?dnt=1",
		"https://vimeo.com/76979871/abc123":                "https://player.vimeo.com/video/76979871?dnt=1&amp;h=abc123",
		"https://player.vimeo.com/video/76979871?h=abc123": "https://player.vimeo.com/video/76979871?dnt=1&amp;h=abc123",
	}
	for in, want := range cases {
		out, _ := render(t, "![A talk]("+in+")")
		contains(t, out, `<iframe class="embed-video" src="`+want+`" title="A talk" loading="lazy"`)
	}
	for _, in := range []string{
		"https://www.youtube.com/watch?v=short",
		"https://www.youtube.com/channel/UC123",
		"https://vimeo.com/channels/staffpicks",
		"https://example.com/watch?v=dQw4w9WgXcQ",
	} {
		out, _ := render(t, "![x]("+in+")")
		if strings.Contains(out, "<iframe") {
			t.Errorf("%s should stay an image: %s", in, out)
		}
	}
	// A link (not an image) to a video stays a link.
	out, _ := render(t, "[talk](https://youtu.be/dQw4w9WgXcQ)")
	if strings.Contains(out, "<iframe") {
		t.Errorf("a plain link became a player: %s", out)
	}
}
