package syncer

import (
	"testing"
	"time"

	"github.com/klppl/kvist/internal/protocol"
)

func TestRegressions(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1, t2 := t0.Add(time.Hour), t0.Add(2*time.Hour)
	f := func(p, h string, mt time.Time) protocol.File { return protocol.File{Path: p, Hash: h, MTime: mt} }
	stored := []protocol.File{
		f(protocol.HintsPath, "h1", t2),
		f("changed-older.md", "a", t1),
		f("changed-newer.md", "a", t1),
		f("removed-new.md", "a", t2),
		f("removed-old.md", "a", t0),
		f("same.md", "a", t2),
	}
	sortFiles(stored)
	incoming := []protocol.File{
		f(protocol.HintsPath, "h2", t0),
		f("added.md", "x", t0),
		f("changed-older.md", "b", t0),
		f("changed-newer.md", "b", t2),
		f("same.md", "a", t0), // identical content, older mtime: fine
	}
	sortFiles(incoming)
	got := Regressions(stored, incoming, t1)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Path != "changed-older.md" || got[0].Kind != protocol.RegressionOlder {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Path != "removed-new.md" || got[1].Kind != protocol.RegressionRemoved {
		t.Errorf("got[1] = %+v", got[1])
	}
	// An unknown client (zero last push) must confirm every removal.
	if got := Regressions(stored, incoming, time.Time{}); len(got) != 3 {
		t.Errorf("unknown client: %+v", got)
	}
}

func sortFiles(fs []protocol.File) {
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && fs[j].Path < fs[j-1].Path; j-- {
			fs[j], fs[j-1] = fs[j-1], fs[j]
		}
	}
}
