package model

import (
	"reflect"
	"strings"
	"testing"
)

func relatedTitles(n *Note) []string {
	var ts []string
	for _, r := range n.Related {
		ts = append(ts, r.Title)
	}
	return ts
}

func TestRelated(t *testing.T) {
	files := map[string]string{
		"Kvist/Kvist.md":         "Welcome to the tomato garden and the sourdough kitchen.",
		"Kvist/Tomatoes.md":      "---\ntags: [garden/vegetables]\n---\nTomatoes need sun, water and staking. Prune tomato suckers weekly.",
		"Kvist/Peppers.md":       "---\ntags: [garden/vegetables]\n---\nPeppers need sun and water like tomatoes, but less staking.",
		"Kvist/Beans.md":         "---\ntags: [garden/legumes]\n---\nBeans climb poles and need water.",
		"Kvist/Sourdough.md":     "---\ntags: [kitchen]\n---\nSourdough bread needs flour, water, salt and a starter.",
		"Kvist/Starter.md":       "---\ntags: [kitchen]\n---\nFeed the starter flour and water daily. See [[Sourdough]].",
		"Kvist/Secret garden.md": "---\nunlisted: true\ntags: [garden/vegetables]\n---\nTomatoes need sun, water and staking. Peppers too.",
		"Kvist/Unrelated.md":     "Quantum chromodynamics describes gluons.",
	}
	site := buildVault(t, files)

	tomatoes := site.Note("Kvist/Tomatoes.md")
	if got := relatedTitles(tomatoes); len(got) == 0 || got[0] != "Peppers" {
		t.Errorf("tomatoes: related = %v, want Peppers first", got)
	}
	for _, n := range site.Notes {
		for _, r := range n.Related {
			switch {
			case r.Unlisted:
				t.Errorf("%s: unlisted %s is related", n.Path, r.Path)
			case r == site.Home:
				t.Errorf("%s: the home note is related", n.Path)
			case r == n:
				t.Errorf("%s: related to itself", n.Path)
			}
		}
		if len(n.Related) != len(n.RelatedIDs) || len(n.Related) > relatedMax {
			t.Errorf("%s: %d related, %d ids", n.Path, len(n.Related), len(n.RelatedIDs))
		}
	}
	if s := site.Note("Kvist/Secret garden.md"); len(s.Related) != 0 {
		t.Errorf("unlisted note has related notes: %v", relatedTitles(s))
	}
	if got := relatedTitles(site.Note("Kvist/Unrelated.md")); len(got) != 0 {
		t.Errorf("a note with nothing in common: related = %v", got)
	}
	// Starter links to Sourdough: they are not suggested to each other.
	for _, p := range []string{"Kvist/Starter.md", "Kvist/Sourdough.md"} {
		for _, title := range relatedTitles(site.Note(p)) {
			if title == "Starter" || title == "Sourdough" {
				t.Errorf("%s: linked note %s suggested as related", p, title)
			}
		}
	}
	// Nested tags count for their parent: Beans shares garden/ with both.
	if got := relatedTitles(site.Note("Kvist/Beans.md")); len(got) == 0 || !strings.Contains(strings.Join(got, ","), "Tomatoes") {
		t.Errorf("beans: related = %v, want Tomatoes", got)
	}

	// Deterministic: the same notes give the same lists.
	for range 5 {
		again := buildVault(t, files)
		for _, n := range site.Notes {
			if a := again.Note(n.Path).RelatedIDs; !reflect.DeepEqual(a, n.RelatedIDs) {
				t.Fatalf("%s: related %v, then %v", n.Path, n.RelatedIDs, a)
			}
		}
	}
}
