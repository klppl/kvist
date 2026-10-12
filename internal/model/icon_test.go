package model

import "testing"

func TestFolderIcon(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/Books/Books.md":     "---\nicon: Book\n---\nAll books.",
		"Kvist/Books/A.md":         "A.",
		"Kvist/Trips/index.md":     "---\nicon: 🧭\n---\nTrips.",
		"Kvist/Trips/A.md":         "A.",
		"Kvist/Ideas/Ideas.md":     "---\nicon: LiBookOpen\n---\nIdeas.",
		"Kvist/Ideas/A.md":         "A.",
		"Kvist/Long/Long.md":       "---\nicon: this is far too long to be an icon of any kind\n---\n.",
		"Kvist/Long/A.md":          "A.",
		"Kvist/Plain/A.md":         "A.",
		"Kvist/Notfolder/Other.md": "---\nicon: star\n---\nNot a folder note.",
	})
	folders := map[string]*Folder{}
	for _, f := range site.Root.Children[0].Children {
		folders[f.Name] = f
	}
	for _, tc := range []struct{ folder, icon, text string }{
		{"Books", "book", ""},
		{"Trips", "", "🧭"},
		{"Ideas", "book-open", ""},
		{"Long", "", ""},
		{"Plain", "", ""},
		{"Notfolder", "", ""},
	} {
		f := folders[tc.folder]
		if f == nil {
			t.Fatalf("no folder %s", tc.folder)
		}
		if f.Icon != tc.icon || f.IconText != tc.text {
			t.Errorf("%s: icon %q, text %q; want %q, %q", tc.folder, f.Icon, f.IconText, tc.icon, tc.text)
		}
	}
	warned := false
	for _, w := range site.Warnings {
		if w.Code == WarnIcon && w.Path == "Kvist/Long/Long.md" {
			warned = true
		}
	}
	if !warned {
		t.Error("no icon warning for Long")
	}
}
