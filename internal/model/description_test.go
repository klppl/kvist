package model

import "testing"

func TestDescriptionKeepsTags(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/Tags.md": "Tags like #garden or #kitchen/bread, but not #public, in a [[Missing|link]].\n\nMore.",
	})
	got := site.Note("Kvist/Tags.md").Description
	if want := "Tags like #garden or #kitchen/bread, but not , in a link."; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}
