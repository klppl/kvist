package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/source/dir"
)

const fixtureConfig = `
data_dir = "unused"
[[site]]
id = "garden"
base_url = "https://garden.example.com"
  [site.publish]
  always_public_folders = ["Garden"]
  exclude_folders = ["Private", "Templates"]
  expose_frontmatter = ["stage"]
`

func writeFixture(t *testing.T, name string) string {
	t.Helper()
	cfg, err := config.Parse([]byte(fixtureConfig))
	if err != nil {
		t.Fatal(err)
	}
	sc := cfg.Sites[0]
	theme, err := LoadTheme("", sc)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := dir.New(filepath.Join("..", "..", "testdata", "leaks", name)).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "site")
	if _, err := WriteSite(context.Background(), sc, theme, snap, out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestNoLeaksInOutput is the leak suite on the final output: no file of the
// built site (HTML, CSS, JS, assets) may contain a private marker.
func TestNoLeaksInOutput(t *testing.T) {
	out := writeFixture(t, "basic")
	var files []string
	err := filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		files = append(files, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if i := bytes.Index(b, []byte("LEAKMARK")); i >= 0 {
			lo, hi := max(0, i-150), min(len(b), i+60)
			t.Errorf("%s leaks a private marker: …%s…", strings.TrimPrefix(p, out), b[lo:hi])
		}
		if strings.Contains(strings.ToLower(p), "leakmark") {
			t.Errorf("output path leaks a private marker: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 10 {
		t.Fatalf("suspiciously few output files: %v", files)
	}
}

func TestSiteOutput(t *testing.T) {
	out := writeFixture(t, "basic")
	for _, p := range []string{
		"index.html", "404.html", "tags/index.html", "tags/garden/index.html", "tags/garden/leaf/index.html",
		"garden/hub/index.html", "garden/leaf/index.html", "garden/index.html",
	} {
		if _, err := os.Stat(filepath.Join(out, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	hub, _ := os.ReadFile(filepath.Join(out, "garden/hub/index.html"))
	for _, frag := range []string{
		`<title>Hub · garden</title>`,
		`<link rel="canonical" href="https://garden.example.com/garden/hub/">`,
		`🌿 budding`,
		`<span class="link-unpublished">a private note</span>`,
		`href="/garden/leaf/"`,
	} {
		if !strings.Contains(string(hub), frag) {
			t.Errorf("hub page lacks %q", frag)
		}
	}
	if bytes.Contains(hub, []byte("<!--")) {
		t.Error("HTML comment in output")
	}
	matches, _ := filepath.Glob(filepath.Join(out, "_assets", "*", "public-pic.png"))
	if len(matches) != 1 {
		t.Errorf("asset not copied: %v", matches)
	}
	if css, _ := filepath.Glob(filepath.Join(out, "_kvist", "*", "style.css")); len(css) != 1 {
		t.Errorf("theme static files not copied: %v", css)
	}

	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(out, p))
		if err != nil {
			t.Errorf("missing %s", p)
		}
		return string(b)
	}
	if s := read("search-index.json"); !strings.Contains(s, `"url":"/garden/hub/"`) || !strings.Contains(s, "A public leaf") {
		t.Errorf("search index: %.300s", s)
	}
	if s := read("graph.json"); !strings.Contains(s, `"edges"`) || !strings.Contains(s, `"garden/leaf"`) {
		t.Errorf("graph: %.300s", s)
	}
	if s := read("index.xml"); !strings.Contains(s, "<link>https://garden.example.com/garden/hub/</link>") || !strings.Contains(s, `<rss version="2.0"`) {
		t.Errorf("feed: %.400s", s)
	}
	if s := read("sitemap.xml"); !strings.Contains(s, "<loc>https://garden.example.com/tags/garden/leaf/</loc>") {
		t.Errorf("sitemap: %.400s", s)
	}
	if s := read("robots.txt"); !strings.Contains(s, "Sitemap: https://garden.example.com/sitemap.xml") {
		t.Errorf("robots: %s", s)
	}
}

func TestIncrementalLinksUnchangedFiles(t *testing.T) {
	cfg, _ := config.Parse([]byte(fixtureConfig))
	sc := cfg.Sites[0]
	theme, err := LoadTheme("", sc)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := dir.New(filepath.Join("..", "..", "testdata", "leaks", "basic")).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	first, second := filepath.Join(base, "a"), filepath.Join(base, "b")
	if _, err := WriteSite(context.Background(), sc, theme, snap, first); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSiteIncremental(context.Background(), sc, theme, snap, second, first); err != nil {
		t.Fatal(err)
	}
	same := func(p string) bool {
		a, err1 := os.Stat(filepath.Join(first, p))
		b, err2 := os.Stat(filepath.Join(second, p))
		return err1 == nil && err2 == nil && os.SameFile(a, b)
	}
	assets, _ := filepath.Glob(filepath.Join(second, "_assets", "*", "*"))
	if len(assets) != 1 || !same(strings.TrimPrefix(assets[0], second+"/")) {
		t.Errorf("asset not linked: %v", assets)
	}
	if !same("garden/hub/index.html") || !same("search-index.json") {
		t.Error("unchanged page not linked")
	}
	// Both builds are complete on their own.
	if err := os.RemoveAll(first); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(second, "garden/hub/index.html")); err != nil || !bytes.Contains(b, []byte("Hub")) {
		t.Error("second build broken after removing the first")
	}
}
