package vault

import (
	"reflect"
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
