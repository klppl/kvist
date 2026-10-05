package render

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/klppl/kvist/internal/model"
)

type memOutput struct {
	files  map[string][]byte
	reused map[string]bool
}

func (m *memOutput) WriteFile(p string, data []byte) error {
	m.files[p] = data
	return nil
}

func (m *memOutput) Reuse(p string) bool { return m.reused[p] }

func TestSocialImages(t *testing.T) {
	site := &model.Site{Config: model.SiteConfig{Title: "My garden", BaseURL: "https://example.com", Params: map[string]any{}}}
	long := strings.Repeat("A rather long title that goes on ", 6)
	site.Notes = []*model.Note{
		{Path: "a.md", URL: "/a/", Title: "Short", Description: "Some words."},
		{Path: "b.md", URL: "/b/", Title: long, Description: strings.Repeat("word ", 80)},
		{Path: "c.md", URL: "/c/", Title: "Supercalifragilisticexpialidocious-and-then-some-more-letters-without-a-break"},
		{Path: "d.md", URL: "/d/", Title: "Own image", Image: "/_assets/x/y.png"},
		{Path: "e.md", URL: "/e/", Title: "東京の庭"},
		{Path: "f.md", URL: "/f/", Title: "Short", Description: "Some words."}, // same picture as a.md
		{Path: "index.md", URL: "/", Title: "Home"},
	}
	out := &memOutput{files: map[string][]byte{}}
	r := &renderer{site: site, theme: garden(t), out: out}
	urls, err := r.socialImages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a.md", "b.md", "c.md"} {
		u := urls[p]
		b := out.files[strings.TrimPrefix(u, "/")]
		if !strings.HasPrefix(u, "/_kvist/social/") || b == nil {
			t.Fatalf("%s: no image (%q)", p, u)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil || img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 630 {
			t.Errorf("%s: not a 1200×630 PNG: %v", p, err)
		}
	}
	for _, p := range []string{"d.md", "e.md", "index.md"} {
		if u, ok := urls[p]; ok {
			t.Errorf("%s should have no generated image, got %s", p, u)
		}
	}
	if urls["a.md"] != urls["f.md"] || len(out.files) != 3 {
		t.Errorf("identical cards should share a file: %v, %d files", urls, len(out.files))
	}

	// The same input gives the same bytes, and a file the previous build
	// has is reused, not drawn again.
	again := &memOutput{files: map[string][]byte{}, reused: map[string]bool{strings.TrimPrefix(urls["a.md"], "/"): true}}
	r.out = again
	urls2, err := r.socialImages()
	if err != nil {
		t.Fatal(err)
	}
	if urls2["b.md"] != urls["b.md"] || !bytes.Equal(again.files[strings.TrimPrefix(urls["b.md"], "/")], out.files[strings.TrimPrefix(urls["b.md"], "/")]) {
		t.Error("social images are not deterministic")
	}
	if len(again.files) != 2 {
		t.Errorf("reused image written again: %d files", len(again.files))
	}

	site.Config.Params["social_images"] = false
	if urls, _ := r.socialImages(); urls != nil {
		t.Errorf("social_images = false still makes images: %v", urls)
	}
}
