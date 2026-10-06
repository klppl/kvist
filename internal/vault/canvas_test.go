package vault

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCanvasLinks(t *testing.T) {
	c, err := ParseCanvas([]byte(`{"nodes":[
		{"id":"a","type":"text","text":"[[One|x]] and ![[pic.png]] and [two](Two.md) %% [[Hidden]] %% ` + "`[[Code]]`" + `"},
		{"id":"b","type":"file","file":"Folder/Note.md","subpath":"#Heading"},
		{"id":"c","type":"link","url":"https://example.com"},
		{"id":"d","type":"group","label":"[[Not a link]]"},
		{"id":"e","type":"file","file":" "}
	],"edges":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got []Link
	for _, l := range c.Links() {
		got = append(got, Link{Target: l.Target, Subpath: l.Subpath, Embed: l.Embed, Markdown: l.Markdown, Offset: l.Offset})
	}
	want := []Link{
		{Target: "One"},
		{Target: "pic.png", Embed: true},
		{Target: "Two.md", Markdown: true},
		{Target: "Folder/Note.md", Subpath: "Heading", Embed: true, Offset: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links = %+v\nwant %+v", got, want)
	}
	if _, err := ParseCanvas([]byte("{")); err == nil {
		t.Error("broken JSON parsed")
	}
	if !IsCanvas("a/B.Canvas") || IsCanvas("a/b.md") {
		t.Error("IsCanvas")
	}
}

// TestCanvasLinksInCode checks that text cards skip links in code and
// comments but not the ones after them; plugin/test/canvas.test.ts has the
// same card.
func TestCanvasLinksInCode(t *testing.T) {
	text := strings.Join([]string{
		"[[Before]]",
		"```js",
		"[[InFence]]",
		"still code ![[in-fence.png]]",
		"```",
		"![[after.png]] and `[[InSpan]]`",
		"~~~~",
		"[[InTilde]]",
		"~~~", // too short to close a ~~~~ fence
		"[[StillInTilde]]",
		"~~~~",
		"%% [[Hidden]] %% <!-- [[AlsoHidden]] -->",
		"[[Last]]",
		"```",
		"[[Unclosed]]",
	}, "\n")
	src, _ := json.Marshal(Canvas{Nodes: []CanvasNode{{ID: "a", Type: "text", Text: text}}})
	c, err := ParseCanvas(src)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range c.Links() {
		got = append(got, l.Target)
	}
	if want := []string{"Before", "after.png", "Last"}; !reflect.DeepEqual(got, want) {
		t.Errorf("links = %q, want %q", got, want)
	}
}
