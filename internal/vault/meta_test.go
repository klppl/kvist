package vault

import (
	"reflect"
	"testing"
)

func TestFrontmatter(t *testing.T) {
	m := ParseMeta([]byte("---\ntitle: Hello\ntags: [a, \"#b\"]\naliases: x, y\n---\nbody #c\n"))
	if m.FrontmatterErr != nil {
		t.Fatal(m.FrontmatterErr)
	}
	if m.Frontmatter["title"] != "Hello" {
		t.Errorf("title = %v", m.Frontmatter["title"])
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(m.Tags, want) {
		t.Errorf("tags = %v, want %v", m.Tags, want)
	}
	if want := []string{"x", "y"}; !reflect.DeepEqual(m.Aliases, want) {
		t.Errorf("aliases = %v, want %v", m.Aliases, want)
	}
}

func TestFrontmatterVariants(t *testing.T) {
	cases := []struct {
		name, src string
		hasFM     bool
		err       bool
	}{
		{"none", "# Title\n", false, false},
		{"crlf", "---\r\npublish: true\r\n---\r\nx", true, false},
		{"bom", "\xef\xbb\xbf---\npublish: true\n---\n", true, false},
		{"unclosed", "---\npublish: true\nno end\n", false, false},
		{"empty", "---\n---\nbody", false, false},
		{"invalid", "---\npublish: [true\n---\n", false, true},
		{"not a map", "---\n- a\n---\n", false, true},
	}
	for _, c := range cases {
		m := ParseMeta([]byte(c.src))
		if got := len(m.Frontmatter) > 0; got != c.hasFM {
			t.Errorf("%s: has frontmatter = %v, want %v", c.name, got, c.hasFM)
		}
		if got := m.FrontmatterErr != nil; got != c.err {
			t.Errorf("%s: err = %v, want err %v", c.name, m.FrontmatterErr, c.err)
		}
	}
}

func TestTags(t *testing.T) {
	src := "" +
		"# Heading #inheading\n" +
		"text #one and #two/nested, #3 #a1 end#no\n" +
		"url http://x.com/#frag \\#escaped\n" +
		"`#incode` and ``#in `double` code``\n" +
		"```\n#fenced\n```\n" +
		"> ```\n> #quotedfence\n> ```\n" +
		"\n    #indented\n\n" +
		"- item\n\n    #listcontinuation\n" +
		"%% #incomment %%\n" +
		"<!-- #inhtml -->\n" +
		"#ÅÄÖ #emoji-ok_1\n"
	m := ParseMeta([]byte(src))
	want := []string{"inheading", "one", "two/nested", "a1", "listcontinuation", "incomment", "inhtml", "ÅÄÖ", "emoji-ok_1"}
	if !reflect.DeepEqual(m.Tags, want) {
		t.Errorf("tags = %q\nwant  %q", m.Tags, want)
	}
}

func TestHasTagIsExactAndCaseInsensitive(t *testing.T) {
	m := ParseMeta([]byte("#Public/sub #PUBLIC2\n"))
	if m.HasTag("public") {
		t.Error("nested tag must not match its parent")
	}
	m = ParseMeta([]byte("#PuBlIc\n"))
	if !m.HasTag("public") {
		t.Error("tags match case-insensitively")
	}
}

func TestLinks(t *testing.T) {
	src := "" +
		"[[Note]] [[Folder/Other#Head|Alias]] ![[img.png|300]] [[#Local]]\n" +
		"[text](Some%20Note.md) ![alt](<attachments/a b.png>) [ext](https://x.com) [mail](mailto:a@b)\n" +
		"`[[incode]]` %% [[incomment]] %% <!-- ![[hidden.png]] -->\n" +
		"[[unclosed\n" +
		"[t](rel/doc.pdf \"title\")\n"
	m := ParseMeta([]byte(src))
	type l struct {
		Target, Subpath, Alias string
		Embed, Markdown        bool
	}
	var got []l
	for _, x := range m.Links {
		got = append(got, l{x.Target, x.Subpath, x.Alias, x.Embed, x.Markdown})
	}
	want := []l{
		{"Note", "", "", false, false},
		{"Folder/Other", "Head", "Alias", false, false},
		{"img.png", "", "300", true, false},
		{"", "Local", "", false, false},
		{"Some Note.md", "", "text", false, true},
		{"attachments/a b.png", "", "alt", true, true},
		{"rel/doc.pdf", "", "t", false, true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links =\n%+v\nwant\n%+v", got, want)
	}
}

func TestUnclosedCommentHidesLinksToEnd(t *testing.T) {
	m := ParseMeta([]byte("[[a]] %% start\n[[b]]\n"))
	if len(m.Links) != 1 || m.Links[0].Target != "a" {
		t.Errorf("links = %+v", m.Links)
	}
}

func TestStripComments(t *testing.T) {
	in := "a %%x%% b\n%%\nblock\n%%\n`%%code%%` <!-- h -->c\n```\n%% fenced %%\n```\n%% open"
	want := "a  b\n\n`%%code%%` c\n```\n%% fenced %%\n```\n"
	if got := string(StripComments([]byte(in))); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
