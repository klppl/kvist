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
}
