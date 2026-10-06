package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRemoveSite(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"work", "garden"} {
		if _, err := st.Site(id); err != nil {
			t.Fatal(err)
		}
	}
	if ids, _ := st.SiteIDs(); !reflect.DeepEqual(ids, []string{"garden", "work"}) {
		t.Fatalf("SiteIDs = %v", ids)
	}
	if err := st.RemoveSite("garden"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "sites", "garden")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("site directory still there: %v", err)
	}
	if ids, _ := st.SiteIDs(); !reflect.DeepEqual(ids, []string{"work"}) {
		t.Errorf("SiteIDs = %v", ids)
	}
	if err := st.RemoveSite("garden"); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing again = %v, want ErrNotFound", err)
	}
	for _, id := range []string{"", ".", "..", "../x", `a\b`} {
		if err := st.RemoveSite(id); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("RemoveSite(%q) = %v, want invalid id", id, err)
		}
	}
	// A removed site comes back empty when it is used again.
	if _, err := st.Site("garden"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "sites", "garden", "revisions")); err != nil {
		t.Error(err)
	}
}
