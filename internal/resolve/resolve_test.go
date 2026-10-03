package resolve

import (
	"testing"

	"github.com/klppl/kvist/internal/vault"
)

func TestResolve(t *testing.T) {
	ix := NewIndex([]string{
		"Garden/Welcome.md",
		"Garden/Note.md",
		"Private/Note.md",
		"Deep/Folder/Note.md",
		"att/map.png",
		"Garden/img/map.png",
		"Other/Thing.md",
	})
	cases := []struct {
		from   string
		link   vault.Link
		want   string
		wantOK bool
	}{
		{"Garden/Welcome.md", vault.Link{Target: "Note"}, "Garden/Note.md", true},          // same folder wins
		{"Other/Thing.md", vault.Link{Target: "note"}, "Garden/Note.md", true},             // shortest, then lexical
		{"Other/Thing.md", vault.Link{Target: "Private/Note"}, "Private/Note.md", true},    // exact path
		{"Other/Thing.md", vault.Link{Target: "Folder/Note"}, "Deep/Folder/Note.md", true}, // suffix
		{"Other/Thing.md", vault.Link{Target: "map.png"}, "att/map.png", true},
		{"Garden/Welcome.md", vault.Link{Target: "img/map.png", Markdown: true}, "Garden/img/map.png", true}, // relative
		{"Garden/Welcome.md", vault.Link{Target: "../Other/Thing.md", Markdown: true}, "Other/Thing.md", true},
		{"Garden/Welcome.md", vault.Link{Target: "Missing"}, "", false},
		{"Garden/Welcome.md", vault.Link{Subpath: "Head"}, "Garden/Welcome.md", true},
		{"Garden/Welcome.md", vault.Link{Target: "../../etc/passwd", Markdown: true}, "", false},
	}
	for _, c := range cases {
		got, ok := ix.Resolve(c.from, c.link)
		if got != c.want || ok != c.wantOK {
			t.Errorf("Resolve(%q, %+v) = %q, %v; want %q, %v", c.from, c.link, got, ok, c.want, c.wantOK)
		}
	}
}
