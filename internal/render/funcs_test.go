package render

import "testing"

func TestHashMod(t *testing.T) {
	if got := hashMod("/notes/a/", 0); got != 0 {
		t.Errorf("n=0: got %d, want 0", got)
	}
	seen := map[int]bool{}
	for _, s := range []string{"/a/", "/b/", "/c/", "/d/", "/e/", "/f/", "/g/", "/h/"} {
		got := hashMod(s, 5)
		if got < 0 || got >= 5 {
			t.Fatalf("hashMod(%q, 5) = %d, out of range", s, got)
		}
		if again := hashMod(s, 5); again != got {
			t.Errorf("hashMod(%q, 5) changed: %d then %d", s, got, again)
		}
		seen[got] = true
	}
	if len(seen) < 2 {
		t.Errorf("hashMod spreads 8 strings over %d values", len(seen))
	}
}
