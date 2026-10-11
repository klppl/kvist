package model

import (
	"reflect"
	"testing"
)

func folderTitles(f *Folder) []string {
	var out []string
	for _, n := range f.Notes {
		out = append(out, n.Title)
	}
	return out
}

func TestFolderSort(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/_site.md": "---\nlanguage: sv\n---\n",
		// Default: newest first, notes with an order ahead of the rest.
		"Kvist/Posts/Alpha.md": "---\nupdated: 2026-10-10\n---\nA.",
		"Kvist/Posts/Beta.md":  "---\nupdated: 2026-10-11\n---\nB.",
		"Kvist/Posts/Gamma.md": "---\nupdated: 2026-10-09\n---\nC.",
		"Kvist/Posts/Start.md": "---\nupdated: 2020-01-01\norder: 2\n---\nS.",
		"Kvist/Posts/First.md": "---\nupdated: 2020-01-01\nweight: \"1\"\n---\nF.",
		// sort: title on the folder note, in the site's language.
		"Kvist/Books/Books.md":   "---\nsort: Title\n---\nAll books.",
		"Kvist/Books/Öar.md":     "Ö.",
		"Kvist/Books/Ängar.md":   "Ä.",
		"Kvist/Books/Åar.md":     "Å.",
		"Kvist/Books/zebra.md":   "Z.",
		"Kvist/Books/Note 10.md": "10.",
		"Kvist/Books/Note 2.md":  "2.",
		// sort: created.
		"Kvist/Log/index.md": "---\nsort: created\n---\nLog.",
		"Kvist/Log/Old.md":   "---\ncreated: 2026-01-01\nupdated: 2026-10-11\n---\n.",
		"Kvist/Log/New.md":   "---\ncreated: 2026-09-01\nupdated: 2026-09-01\n---\n.",
		// An unknown sort value falls back to the default, with a warning.
		"Kvist/Odd/Odd.md": "---\nsort: random\n---\n.",
		"Kvist/Odd/A.md":   "---\nupdated: 2026-01-01\n---\n.",
		"Kvist/Odd/B.md":   "---\nupdated: 2026-02-01\n---\n.",
	})
	folders := map[string]*Folder{}
	for _, f := range site.Root.Children[0].Children {
		folders[f.Name] = f
	}

	if got, want := folderTitles(folders["Posts"]), []string{"First", "Start", "Beta", "Alpha", "Gamma"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Posts: %v, want %v", got, want)
	}
	if folders["Posts"].Sort != "updated" {
		t.Errorf("Posts sort: %q", folders["Posts"].Sort)
	}
	if o := site.Note("Kvist/Posts/First.md").Order; o == nil || *o != 1 {
		t.Errorf("weight as text: %v", o)
	}

	// Books.md is the folder's page, not one of its notes.
	if got, want := folderTitles(folders["Books"]), []string{"Note 2", "Note 10", "zebra", "Åar", "Ängar", "Öar"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Books: %v, want %v", got, want)
	}
	if folders["Books"].Sort != "title" {
		t.Errorf("Books sort: %q", folders["Books"].Sort)
	}

	if got, want := folderTitles(folders["Log"]), []string{"New", "Old"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Log: %v, want %v", got, want)
	}

	if got, want := folderTitles(folders["Odd"]), []string{"B", "A"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Odd: %v, want %v", got, want)
	}
	warned := false
	for _, w := range site.Warnings {
		if w.Code == WarnSort && w.Path == "Kvist/Odd/Odd.md" {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no warning for sort: random: %v", site.Warnings)
	}
}

// TestFolderNoteOnly checks that a folder holding only its folder note
// keeps its place in the tree, with no notes of its own.
func TestFolderNoteOnly(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/About/index.md":  "About.",
		"Kvist/Hidden/index.md": "---\nunlisted: true\n---\nHidden.",
	})
	var names []string
	for _, f := range site.Root.Children[0].Children {
		names = append(names, f.Name)
		if len(f.Notes) != 0 || f.Index == nil {
			t.Errorf("%s: %d notes, index %v", f.Name, len(f.Notes), f.Index)
		}
	}
	if !reflect.DeepEqual(names, []string{"About"}) {
		t.Errorf("folders: %v, want [About]", names)
	}
}
