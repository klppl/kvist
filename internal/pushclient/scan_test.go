package pushclient

import (
	"path/filepath"
	"testing"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/protocol"
)

// TestScanCanvases checks gate 1 for canvases on the leak fixture: a canvas
// goes up when it is in an always-public folder or a published note links
// to it, with the attachments its cards show, and never from an excluded
// folder.
func TestScanCanvases(t *testing.T) {
	rules := protocol.Rules{
		AlwaysPublicFolders:  []string{"Garden"},
		ExcludeFolders:       []string{"Private", "Templates"},
		PublicTag:            "public",
		PrivateTag:           "private",
		FrontmatterKey:       "publish",
		AttachmentExtensions: config.DefaultAttachmentExtensions,
	}
	s, err := ScanDir(filepath.Join("..", "..", "testdata", "leaks", "basic"), rules)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range s.Files {
		got[f.Path] = true
	}
	for _, p := range []string{"Garden/Board.canvas", "Journal/Linked.canvas", "attachments/public-pic.png"} {
		if !got[p] {
			t.Errorf("%s not pushed", p)
		}
	}
	for _, p := range []string{"Private/LEAKMARK-board.canvas", "Journal/LEAKMARK-loose.canvas", "Private/LEAKMARK-image.png", "attachments/LEAKMARK-unreferenced.png", "attachments/LEAKMARK-commentref.png"} {
		if got[p] {
			t.Errorf("%s pushed", p)
		}
	}
	reported := false
	for _, l := range s.UnpublishedLinks {
		reported = reported || (l.From == "Journal/Linked.canvas" && l.Target == "Journal/LEAKMARK-untagged.md" && l.Embed)
	}
	if !reported {
		t.Errorf("the linked canvas's private card is not reported: %+v", s.UnpublishedLinks)
	}
}
