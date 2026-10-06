package pushclient

import (
	"io"
	"path/filepath"
	"strings"
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

// TestScanStripsPrivateCards checks that a canvas goes up without the file
// cards of notes and files that stay private, and without their arrows, so
// their vault paths never reach the server.
func TestScanStripsPrivateCards(t *testing.T) {
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
	read := func(p string) string {
		t.Helper()
		r, err := s.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range s.Files {
			if f.Path == p && (f.Hash != protocol.HashBytes(b) || f.Size != int64(len(b))) {
				t.Errorf("%s: the manifest doesn't describe what is sent", p)
			}
		}
		return string(b)
	}

	board := read("Garden/Board.canvas")
	for _, gone := range []string{"LEAKMARK-path-secret.md", "LEAKMARK-image.png", "LEAKMARK-board.canvas", "LEAKMARK-id-secret", "LEAKMARK-id-image", "LEAKMARK-id-board", "LEAKMARK-edge-label", "LEAKMARK-edge-image"} {
		if strings.Contains(board, gone) {
			t.Errorf("Board.canvas carries %q:\n%s", gone, board)
		}
	}
	for _, kept := range []string{`"file":"Garden/Leaf.md"`, `"label":"Board edge"`, `"label":"Board group"`, "Board text: [[Leaf]]"} {
		if !strings.Contains(board, kept) {
			t.Errorf("Board.canvas lost %q:\n%s", kept, board)
		}
	}
	if linked := read("Journal/Linked.canvas"); strings.Contains(linked, "LEAKMARK") {
		t.Errorf("Linked.canvas carries a private name:\n%s", linked)
	}
}

func TestStripCanvas(t *testing.T) {
	keepAll := func(string) bool { return true }
	src := []byte("{\n\t\"nodes\":[{\"id\":\"a\",\"type\":\"file\",\"file\":\"A.md\"}],\n\t\"edges\":[]\n}")
	if out, ok := stripCanvas(src, keepAll); !ok || string(out) != string(src) {
		t.Errorf("a canvas with nothing to strip changed: %q", out)
	}
	src = []byte(`{"nodes":[{"id":"a","type":"file","file":"A.md","x":1.5},{"id":"b","type":"file","file":"B.md"},{"id":"c","type":"text","text":"<b>&</b>"}],` +
		`"edges":[{"id":"e1","fromNode":"a","toNode":"b"},{"id":"e2","fromNode":"c","toNode":"a"}],"extra":true}`)
	out, ok := stripCanvas(src, func(f string) bool { return f != "B.md" })
	want := `{"edges":[{"fromNode":"c","id":"e2","toNode":"a"}],"extra":true,"nodes":[{"file":"A.md","id":"a","type":"file","x":1.5},{"id":"c","text":"<b>&</b>","type":"text"}]}`
	if !ok || string(out) != want {
		t.Errorf("stripCanvas = %s, %v; want %s", out, ok, want)
	}
	// The same canvas as in the plugin's scan test, which expects these bytes.
	src = []byte(`{"nodes":[{"id":"t","type":"text","text":"<b>[[Leaf]]</b>"},{"id":"leaf","type":"file","file":"Garden/Leaf.md","x":1.5},` +
		`{"id":"diary","type":"file","file":"Journal/Diary.md"},{"id":"img","type":"file","file":"Private/secret.png"},{"id":"gone","type":"file","file":"Missing.md"}],` +
		`"edges":[{"id":"e1","fromNode":"t","toNode":"leaf","label":"kept"},{"id":"e2","fromNode":"leaf","toNode":"diary","label":"SECRET-edge"},{"id":"e3","fromNode":"img","toNode":"t"}],"extra":true}`)
	out, _ = stripCanvas(src, func(f string) bool { return f == "Garden/Leaf.md" })
	want = `{"edges":[{"fromNode":"t","id":"e1","label":"kept","toNode":"leaf"}],"extra":true,` +
		`"nodes":[{"id":"t","text":"<b>[[Leaf]]</b>","type":"text"},{"file":"Garden/Leaf.md","id":"leaf","type":"file","x":1.5}]}`
	if string(out) != want {
		t.Errorf("stripCanvas = %s; the plugin sends %s", out, want)
	}
	if _, ok := stripCanvas([]byte("not json"), keepAll); ok {
		t.Error("stripCanvas accepted a file that isn't JSON")
	}
}
