package model

import (
	"strings"
	"testing"
)

func TestCanvasFixture(t *testing.T) {
	site := buildFixture(t, "basic", nil)
	board := site.Note("Garden/Board.canvas")
	if board == nil || !board.Canvas || board.Title != "Board" || board.URL != "/garden/board-canvas/" {
		t.Fatalf("board = %+v", board)
	}
	html := string(board.Content)
	for _, want := range []string{
		`<div class="canvas"><div class="canvas-board"`,
		`<div class="canvas-group-label">Board group</div>`,
		`<a class="internal-link" href="/garden/leaf/">Leaf</a>`, // text card link and file card title
		`<span class="link-unpublished">hidden note</span>`,      // private link: text, as in notes
		`<img src="/_assets/`,                                    // public image embed in a text card
		`<div class="canvas-edge-label" style="left:`,            // the edge between two shown cards
		`>Board edge</div>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("board lacks %q:\n%s", want, html)
		}
	}
	// Three of six cards are left out (private note, excluded image, private
	// canvas), and with them the arrows to them.
	if n := strings.Count(html, `class="canvas-node `); n != 3 {
		t.Errorf("%d cards, want 3 (group, text, leaf):\n%s", n, html)
	}
	if n := strings.Count(html, `<g class="canvas-edge`); n != 1 {
		t.Errorf("%d arrows, want 1:\n%s", n, html)
	}
	if !strings.Contains(board.Text, "Board text") || !strings.Contains(board.Text, "Board edge") {
		t.Errorf("search text = %q", board.Text)
	}

	// Linked from a published note, so published; its card showing a private
	// note is gone.
	linked := site.Note("Journal/Linked.canvas")
	if linked == nil || strings.Count(string(linked.Content), `class="canvas-node `) != 1 {
		t.Errorf("linked canvas = %+v", linked)
	}

	boards := site.Note("Garden/Boards.md")
	for _, want := range []string{
		`<a class="internal-link" href="/garden/board-canvas/">Board.canvas</a>`,
		`<div class="embed"><div class="embed-title"><a class="internal-link" href="/garden/board-canvas/">Board</a></div><div class="embed-content"><div class="canvas">`,
		`<span class="link-unpublished">private board</span>`,
	} {
		if !strings.Contains(string(boards.Content), want) {
			t.Errorf("boards note lacks %q:\n%s", want, boards.Content)
		}
	}
	for _, a := range site.Assets {
		if strings.HasSuffix(a.Path, ".canvas") {
			t.Errorf("canvas file published as an asset: %s", a.Path)
		}
	}
}

func TestCanvasLayout(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/B.canvas": `{"nodes":[
			{"id":"a","type":"text","text":"A","x":-100,"y":-50,"width":200,"height":100,"color":"3"},
			{"id":"b","type":"text","text":"B","x":300,"y":-50,"width":200,"height":100,"color":"#00ff00"},
			{"id":"c","type":"link","url":"https://example.com/x","x":-100,"y":200,"width":200,"height":60,"color":"red;}"},
			{"id":"d","type":"link","url":"javascript:alert(1)","x":300,"y":200,"width":200,"height":60},
			{"id":"e","type":"text","text":"zero","x":0,"y":0,"width":0,"height":10}
		],"edges":[
			{"id":"1","fromNode":"a","fromSide":"right","toNode":"b","toSide":"left","color":"1"},
			{"id":"2","fromNode":"a","toNode":"c","fromEnd":"arrow","toEnd":"none"},
			{"id":"3","fromNode":"a","toNode":"d"}
		]}`,
	})
	html := string(site.Note("Kvist/B.canvas").Content)
	for _, want := range []string{
		// Board: the cards' extent (-100…500 × -50…260) plus 40px around.
		`<div class="canvas-board" style="width:680px;height:390px">`,
		`<div class="canvas-node canvas-text canvas-color-3" style="left:40px;top:40px;width:200px;height:100px;">`,
		`<div class="canvas-node canvas-text canvas-color" style="left:440px;top:40px;width:200px;height:100px;--canvas-color:#00ff00;">`,
		// An unknown color is no color.
		`<div class="canvas-node canvas-link" style="left:40px;top:290px;width:200px;height:60px;">`,
		`<a class="external-link" href="https://example.com/x" rel="noopener">example.com/x</a>`,
		// Right side of a to left side of b, curving out horizontally.
		`<g class="canvas-edge canvas-color-1"><path d="M240 90 C340 90 340 90 440 90"/><path class="canvas-arrow" d="M440 90 L428 96 L428 84 Z"/></g>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("lacks %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "javascript") || strings.Contains(html, "zero") {
		t.Errorf("unsafe link or empty card shown:\n%s", html)
	}
	// Edge 2 has its arrow at the start only; edge 3 goes to a card that
	// was left out.
	if n := strings.Count(html, `class="canvas-arrow"`); n != 2 {
		t.Errorf("%d arrowheads, want 2:\n%s", n, html)
	}
	if n := strings.Count(html, `<g class="canvas-edge`); n != 2 {
		t.Errorf("%d arrows, want 2:\n%s", n, html)
	}
}

func TestCanvasPublishing(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/Note.md": "![[Embedded.canvas]]",
		"Notes/Embedded.canvas": `{"nodes":[
			{"id":"a","type":"text","text":"See [[Chained.canvas]]","x":0,"y":0,"width":100,"height":100},
			{"id":"b","type":"file","file":"Kvist/Note.md","x":200,"y":0,"width":100,"height":100}
		]}`,
		"Notes/Chained.canvas": `{"nodes":[{"id":"a","type":"text","text":"$x^2$","x":0,"y":0,"width":100,"height":100}]}`,
		"Notes/Alone.canvas":   `{"nodes":[]}`,
		"Kvist/Broken.canvas":  `{"nodes":`,
	})
	var paths []string
	for _, n := range site.Notes {
		paths = append(paths, n.Path)
	}
	if got := strings.Join(paths, ","); got != "Kvist/Note.md,Notes/Chained.canvas,Notes/Embedded.canvas" {
		t.Errorf("published = %s", got)
	}
	// The embedded canvas shows the note that embeds it as a card, but
	// that card doesn't embed the canvas again.
	note := string(site.Note("Kvist/Note.md").Content)
	if strings.Count(note, `class="canvas"`) != 1 {
		t.Errorf("note:\n%s", note)
	}
	if !site.Note("Notes/Chained.canvas").Features.Math {
		t.Error("math in a text card does not load math")
	}
	found := false
	for _, w := range site.Warnings {
		found = found || (w.Code == WarnCanvas && w.Path == "Kvist/Broken.canvas")
	}
	if !found {
		t.Errorf("no warning for the broken canvas: %+v", site.Warnings)
	}
}
