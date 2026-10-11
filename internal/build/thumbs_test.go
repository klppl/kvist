package build

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/source/dir"
)

func testPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}
	img.Set(0, 0, color.RGBA{200, 30, 30, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestThumbnails checks that a note's image gets small copies for the list
// pane and cards, and that an SVG is used as it is.
func TestThumbnails(t *testing.T) {
	out := writeVault(t, map[string]string{
		"Garden/Photo.md":    "---\ncover: \"[[photo.png]]\"\n---\nPhoto text.",
		"Garden/Drawing.md":  "---\nimage: \"[[drawing.svg]]\"\n---\nDrawing text.",
		"Garden/Plain.md":    "Plain text.",
		"Garden/photo.png":   testPNG(t, 900, 600),
		"Garden/drawing.svg": `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`,
	})
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(out, p))
		if err != nil {
			t.Fatalf("missing %s", p)
		}
		return string(b)
	}
	thumbs, _ := filepath.Glob(filepath.Join(out, "_assets", "*", "photo-thumb.jpg"))
	cards, _ := filepath.Glob(filepath.Join(out, "_assets", "*", "photo-card.jpg"))
	if len(thumbs) != 1 || len(cards) != 1 {
		t.Fatalf("thumbnails: %v %v", thumbs, cards)
	}
	for p, want := range map[string]image.Point{thumbs[0]: {128, 128}, cards[0]: {640, 360}} {
		b, _ := os.ReadFile(p)
		img, err := jpeg.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if got := img.Bounds().Size(); got != want {
			t.Errorf("%s is %v, want %v", filepath.Base(p), got, want)
		}
	}
	thumbURL := "/" + filepath.ToSlash(strings.TrimPrefix(thumbs[0], out+string(filepath.Separator)))
	cardURL := "/" + filepath.ToSlash(strings.TrimPrefix(cards[0], out+string(filepath.Separator)))

	// The list pane on a note page shows the thumbnails.
	page := read("garden/plain/index.html")
	if !strings.Contains(page, `class="list-thumb" src="`+thumbURL+`"`) {
		t.Error("list pane lacks the photo's thumbnail")
	}
	if !regexp.MustCompile(`class="list-thumb" src="/_assets/[0-9a-f]{8}/drawing\.svg"`).MatchString(page) {
		t.Error("list pane lacks the SVG as it is")
	}
	if n := strings.Count(page, `class="list-thumb"`); n != 2 {
		t.Errorf("%d thumbnails in the list pane, want 2 (none for the plain note)", n)
	}
	// Folder cards show the wide copy.
	if !strings.Contains(read("garden/index.html"), `class="card-cover" src="`+cardURL+`"`) {
		t.Error("folder card lacks the photo's cover")
	}
	// The social preview stays the full image.
	if !regexp.MustCompile(`og:image" content="https://garden\.example\.com/_assets/[0-9a-f]{8}/photo\.png"`).MatchString(read("garden/photo/index.html")) {
		t.Error("og:image is not the full image")
	}
}

func TestThumbnailsOff(t *testing.T) {
	out := writeVault(t, map[string]string{
		"_site.md":         "---\nlist_images: false\n---\n",
		"Garden/Photo.md":  "---\ncover: \"[[photo.png]]\"\n---\nPhoto text.",
		"Garden/Plain.md":  "Plain text.",
		"Garden/photo.png": testPNG(t, 300, 200),
	})
	b, err := os.ReadFile(filepath.Join(out, "garden/plain/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "list-thumb") {
		t.Error("list_images = false still shows thumbnails")
	}
}

// TestThumbnailsReused checks that an incremental build links unchanged
// thumbnails from the previous build instead of making them again.
func TestThumbnailsReused(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "Garden"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "Garden", "Photo.md"), []byte("---\ncover: \"[[photo.png]]\"\n---\nText."), 0o644)
	os.WriteFile(filepath.Join(src, "Garden", "photo.png"), []byte(testPNG(t, 400, 300)), 0o644)
	cfg, _ := config.Parse([]byte(fixtureConfig))
	sc := cfg.Sites[0]
	theme, err := LoadTheme("", sc)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := dir.New(src).Snapshot(context.Background())
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
	for _, name := range []string{"photo-thumb.jpg", "photo-card.jpg"} {
		m, _ := filepath.Glob(filepath.Join(second, "_assets", "*", name))
		if len(m) != 1 {
			t.Fatalf("%s: %v", name, m)
		}
		rel, _ := filepath.Rel(second, m[0])
		a, err1 := os.Stat(filepath.Join(first, rel))
		b, err2 := os.Stat(m[0])
		if err1 != nil || err2 != nil || !os.SameFile(a, b) {
			t.Errorf("%s not linked from the previous build", name)
		}
	}
}
