package vault

import (
	"encoding/json"
	"os"
	"testing"
)

func TestImageValueParity(t *testing.T) {
	data, err := os.ReadFile("../../testdata/parity/images.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []struct {
			Value  any    `json:"value"`
			Target string `json:"target"`
			URL    string `json:"url"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatal(err)
	}
	for _, c := range fx.Cases {
		r, ok := ParseImageValue(c.Value)
		want := c.Target != "" || c.URL != ""
		if ok != want || r.Link.Target != c.Target || r.URL != c.URL {
			t.Errorf("%#v: got %+v (%v), want target %q url %q", c.Value, r, ok, c.Target, c.URL)
		}
	}
}

func TestImageProperty(t *testing.T) {
	fm := map[string]any{"Cover": "[[c.png]]", "image": ""}
	if r, ok := ImageProperty(fm, NoteImageKeys); !ok || r.Link.Target != "c.png" {
		t.Errorf("an empty image should fall through to Cover: %+v %v", r, ok)
	}
	fm["image"] = "i.png"
	if r, _ := ImageProperty(fm, NoteImageKeys); r.Link.Target != "i.png" {
		t.Errorf("image comes before cover: %+v", r)
	}
	if _, ok := ImageProperty(fm, SiteAvatarKeys); ok {
		t.Error("no avatar set")
	}
	for p, want := range map[string]bool{"a/b.PNG": true, "a.jpg": true, "a.pdf": false, "a.png/b": false, "png": false, "x/.png": false} {
		if IsImage(p) != want {
			t.Errorf("IsImage(%q) != %v", p, want)
		}
	}
}
