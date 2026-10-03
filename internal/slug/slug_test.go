package slug

import "testing"

func TestMake(t *testing.T) {
	cases := map[string]string{
		"My Note":            "my-note",
		"  Hello, World!  ":  "hello-world",
		"Räksmörgås & co":    "räksmörgås-co",
		"C++ / Go (2026)":    "c-go-2026",
		"---":                "",
		"Café":              "café",
		"already-slugged_ok": "already-slugged-ok",
	}
	for in, want := range cases {
		if got := Make(in); got != want {
			t.Errorf("Make(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Path("Garden/My Note.md"); got != "garden/my-note" {
		t.Errorf("Path = %q", got)
	}
	var u Unique
	for _, want := range []string{"intro", "intro-1", "intro-2", "section"} {
		in := "Intro"
		if want == "section" {
			in = "!!"
		}
		if got := u.Get(in); got != want {
			t.Errorf("Unique.Get = %q, want %q", got, want)
		}
	}
}
