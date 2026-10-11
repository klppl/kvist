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
root_folder = "/" # keep the folder in addresses (root_test.go covers root folders)
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
		// a generated social preview image
		`<meta property="og:image" content="https://garden.example.com/_kvist/social/`,
		`<meta name="twitter:card" content="summary_large_image">`,
		// breadcrumbs
		`<nav class="crumbs" aria-label="Folder path"><a href="/">Home</a><span aria-hidden="true">/</span><a href="/garden/">Garden</a></nav>`,
		// search engine metadata
		`<meta name="robots" content="max-image-preview:large">`,
		`<meta property="og:type" content="article">`,
		`<meta property="article:published_time" content="`,
		`<script type="application/ld+json">[{"@context":"https://schema.org","@type":"Article",`,
		`"headline":"Hub"`,
		`{"@type":"ListItem","item":"https://garden.example.com/garden/","name":"Garden","position":2}`,
	} {
		if !strings.Contains(string(hub), frag) {
			t.Errorf("hub page lacks %q", frag)
		}
	}
	// The settings note's image is the default social preview image of
	// the other pages.
	if home, _ := os.ReadFile(filepath.Join(out, "index.html")); !bytes.Contains(home, []byte(`<meta property="og:image" content="https://garden.example.com/_assets/`)) {
		t.Error("home page lacks the site's image")
	}
	if home, _ := os.ReadFile(filepath.Join(out, "index.html")); !bytes.Contains(home, []byte(`<meta property="og:type" content="website">`)) || !bytes.Contains(home, []byte(`"@type":"WebSite"`)) {
		t.Error("home page is not described as the website")
	}
	if nf, _ := os.ReadFile(filepath.Join(out, "404.html")); !bytes.Contains(nf, []byte(`<meta name="robots" content="noindex">`)) || bytes.Contains(nf, []byte("ld+json")) {
		t.Error("404 page should be noindex, without structured data")
	}
	if social, _ := filepath.Glob(filepath.Join(out, "_kvist", "social", "*.png")); len(social) == 0 {
		t.Error("no social images written")
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
	// Folder and tag pages carry the date of their newest note.
	if s := read("sitemap.xml"); !strings.Contains(s, "<loc>https://garden.example.com/tags/garden/leaf/</loc>\n    <lastmod>") ||
		!strings.Contains(s, "<loc>https://garden.example.com/tags/</loc>\n    <lastmod>") {
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
	if _, _, err := WriteSiteIncremental(context.Background(), sc, theme, snap, second, first, nil); err != nil {
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

// writeVault builds a site from the given files with the fixture config
// and returns the output directory.
func writeVault(t *testing.T, files map[string]string) string {
	t.Helper()
	src := t.TempDir()
	for p, c := range files {
		full := filepath.Join(src, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Parse([]byte(fixtureConfig))
	if err != nil {
		t.Fatal(err)
	}
	sc := cfg.Sites[0]
	theme, err := LoadTheme("", sc)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := dir.New(src).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "site")
	if _, err := WriteSite(context.Background(), sc, theme, snap, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUnlistedOutput(t *testing.T) {
	out := writeVault(t, map[string]string{
		"Garden/Listed.md": "Listed text. See [[Hidden]].",
		"Garden/Hidden.md": "---\nunlisted: true\n---\nHiddenword text.",
	})
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(out, p))
		if err != nil {
			t.Fatalf("missing %s", p)
		}
		return string(b)
	}
	hidden := read("garden/hidden/index.html")
	if !strings.Contains(hidden, `<meta name="robots" content="noindex">`) {
		t.Error("unlisted page lacks noindex")
	}
	if strings.Contains(read("garden/listed/index.html"), "noindex") {
		t.Error("listed page has noindex")
	}
	for _, p := range []string{"search-index.json", "graph.json", "index.xml", "sitemap.xml", "garden/index.html", "index.html", "tags/index.html"} {
		if s := read(p); strings.Contains(s, "garden/hidden") || strings.Contains(s, "Hiddenword") {
			t.Errorf("%s lists the unlisted note", p)
		}
	}
}

// TestHomeNoteNotInRecent checks that a note set as home in the settings
// note is the home page and not also an entry in its recent list.
func TestHomeNoteNotInRecent(t *testing.T) {
	out := writeVault(t, map[string]string{
		"_site.md":        "---\nhome: \"[[Start]]\"\n---\n",
		"Garden/Start.md": "Startword text.",
		"Garden/Other.md": "Other text.",
	})
	b, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	home := string(b)
	if !strings.Contains(home, "Startword") {
		t.Error("home page lacks the home note's text")
	}
	if !strings.Contains(home, `href="/garden/other/"`) {
		t.Error("recent list lacks the other note")
	}
	if strings.Contains(home, `href="/garden/start/"`) {
		t.Error("recent list includes the home note")
	}
}

// TestFolderNoteNotInRecent checks that a folder note is the folder's
// page and not also an entry in the home page's recent list.
func TestFolderNoteNotInRecent(t *testing.T) {
	out := writeVault(t, map[string]string{
		"Garden/index.md": "Folderword text.",
		"Garden/Other.md": "Other text.",
	})
	b, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	home := string(b)
	if !strings.Contains(home, `href="/garden/other/"`) {
		t.Error("recent list lacks the other note")
	}
	if strings.Contains(home, "Folderword") {
		t.Error("recent list includes the folder note")
	}
}

// TestStackedPagesSetting checks that the garden theme marks pages for
// stacked pages only when the setting is on (garden.js does the rest).
func TestStackedPagesSetting(t *testing.T) {
	note := "Garden/Note.md"
	for _, tc := range []struct {
		settings string
		want     bool
	}{
		{"", false},
		{"---\nstacked_pages: true\n---\n", true},
		{"---\nstacked_pages: false\n---\n", false},
	} {
		files := map[string]string{note: "Text with a [[Other]] link.", "Garden/Other.md": "Other text."}
		if tc.settings != "" {
			files["_site.md"] = tc.settings
		}
		out := writeVault(t, files)
		b, err := os.ReadFile(filepath.Join(out, "garden/note/index.html"))
		if err != nil {
			t.Fatal(err)
		}
		if got := bytes.Contains(b, []byte(" data-stacked")); got != tc.want {
			t.Errorf("settings %q: data-stacked = %v, want %v", tc.settings, got, tc.want)
		}
	}
}
