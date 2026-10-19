package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLayout(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/Plain.md":    "Plain text.",
		"Kvist/Article.md":  "---\nlayout: Article\n---\nText.",
		"Kvist/Default.md":  "---\nlayout: default\n---\nText.",
		"Kvist/About.md":    "---\nlayout: About\ndescription: Gardener.\n---\nFirst paragraph.",
		"Kvist/Intro.md":    "---\nlayout: intro\n---\nFirst paragraph.",
		"Kvist/Page.md":     "---\nlayout: page\nsummary: A page.\n---\nText.",
		"Kvist/Cover.md":    "---\nlayout: cover\n---\nNo picture.",
		"Kvist/Typo.md":     "---\nlayout: profil\n---\nText.",
		"Kvist/Jekyll.md":   "---\nlayout: post\n---\nText.",
		"Kvist/Unknown.md":  "---\nlayout: [a, b]\n---\nText.",
		"Kvist/Pictured.md": "---\nlayout: cover\nimage: https://example.com/a.png\n---\nText.",
	})
	notes := map[string]*Note{}
	for _, n := range site.Notes {
		notes[n.Path] = n
	}
	for _, tc := range []struct{ path, layout, lead string }{
		{"Kvist/Plain.md", "", ""},
		{"Kvist/Article.md", "", ""},
		{"Kvist/Default.md", "", ""},
		{"Kvist/About.md", "about", "Gardener."},
		{"Kvist/Intro.md", "intro", ""},
		{"Kvist/Page.md", "page", "A page."},
		{"Kvist/Cover.md", "cover", ""},
		{"Kvist/Typo.md", "", ""},
		{"Kvist/Jekyll.md", "", ""},
		{"Kvist/Unknown.md", "", ""},
		{"Kvist/Pictured.md", "cover", ""},
	} {
		n := notes[tc.path]
		if n == nil {
			t.Fatalf("%s missing", tc.path)
		}
		if n.Layout != tc.layout || n.Lead != tc.lead {
			t.Errorf("%s: layout %q, lead %q; want %q, %q", tc.path, n.Layout, n.Lead, tc.layout, tc.lead)
		}
	}
	// The lead is only what the frontmatter says; the description falls
	// back to the first paragraph as before.
	if d := notes["Kvist/Intro.md"].Description; d != "First paragraph." {
		t.Errorf("intro description = %q", d)
	}

	warned := map[string]string{}
	for _, w := range site.Warnings {
		if w.Code == WarnLayout {
			warned[w.Path] = w.Message
		}
	}
	for _, p := range []string{"Kvist/Typo.md", "Kvist/Jekyll.md", "Kvist/Unknown.md", "Kvist/Cover.md"} {
		if warned[p] == "" {
			t.Errorf("%s: no layout warning", p)
		}
	}
	if !strings.Contains(warned["Kvist/Typo.md"], `"profil"`) || !strings.Contains(warned["Kvist/Typo.md"], "about") {
		t.Errorf("typo warning doesn't name the value and the choices: %q", warned["Kvist/Typo.md"])
	}
	for _, p := range []string{"Kvist/Plain.md", "Kvist/Article.md", "Kvist/Default.md", "Kvist/About.md", "Kvist/Pictured.md"} {
		if m := warned[p]; m != "" {
			t.Errorf("%s: unexpected warning %q", p, m)
		}
	}
}

// TestLayoutParity runs the fixture the plugin's tests run too
// (plugin/src/layouts.ts), so both read the layout property alike.
func TestLayoutParity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "parity", "layouts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Layouts []string `json:"layouts"`
		Cases   []struct {
			Value  any     `json:"value"`
			Layout *string `json:"layout"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fx.Layouts, Layouts) {
		t.Errorf("Layouts = %v, fixture %v", Layouts, fx.Layouts)
	}
	for _, c := range fx.Cases {
		b := &builder{}
		got := b.layout("n.md", map[string]any{"layout": c.Value})
		if c.Layout == nil {
			if len(b.warnings) != 1 || got != "" {
				t.Errorf("%v: layout %q, warnings %v; want ignored with a warning", c.Value, got, b.warnings)
			}
		} else if got != *c.Layout || len(b.warnings) != 0 {
			t.Errorf("%v: layout %q, warnings %v; want %q", c.Value, got, b.warnings, *c.Layout)
		}
	}
}
