package publish

import (
	"testing"

	"github.com/klppl/kvist/internal/protocol"
)

func TestEvaluate(t *testing.T) {
	r := protocol.Rules{
		AlwaysPublicFolders: []string{"Garden"},
		ExcludeFolders:      []string{"Private", "Garden/Drafts"},
		PublicTag:           "public",
		PrivateTag:          "private",
		FrontmatterKey:      "publish",
	}
	cases := []struct {
		path, src string
		want      bool
		reason    string
	}{
		{"Garden/a.md", "hi", true, ReasonPublicFolder},
		{"Garden/Drafts/a.md", "#public", false, ReasonExcludedFolder},
		{"private/a.md", "#public", false, ReasonExcludedFolder}, // case-insensitive exclusion
		{"garden/a.md", "hi", false, ReasonNoRule},               // case-sensitive inclusion
		{"Garden/a.md", "#private", false, ReasonPrivateTag},
		{"Garden/a.md", "---\npublish: false\n---\n", false, ReasonFrontmatterFalse},
		{"Garden/a.md", "---\nPublish: \"false\"\n---\n", false, ReasonFrontmatterFalse},
		{"x.md", "#public", true, ReasonPublicTag},
		{"x.md", "#Public", true, ReasonPublicTag},
		{"x.md", "#public/sub", false, ReasonNoRule},
		{"x.md", "`#public`", false, ReasonNoRule},
		{"x.md", "#public %% #private %%", false, ReasonPrivateTag},
		{"x.md", "---\npublish: true\n---\n", true, ReasonFrontmatterTrue},
		{"x.md", "---\npublish: yes\n---\n", false, ReasonNoRule},
		{"x.md", "---\ntags: [public]\n---\n", true, ReasonPublicTag},
		{"x.md", "---\npublish: true\npublish: false\n---\n", false, ReasonInvalidFrontmatter},
		{"x.md", "---\npublish: [\n---\n#public", false, ReasonInvalidFrontmatter},
		{"x.md", "---\npublish: true\nPublish: false\n---\n", false, ReasonFrontmatterFalse},
		{"x.png", "", false, ReasonNotANote},
		{"Gardener/a.md", "", false, ReasonNoRule},
	}
	for _, c := range cases {
		d, _ := EvaluateSource(r, c.path, []byte(c.src))
		if d.Published != c.want || d.Reason != c.reason {
			t.Errorf("%s %q: got %+v, want %v/%s", c.path, c.src, d, c.want, c.reason)
		}
	}
}

func TestRootFolder(t *testing.T) {
	r := protocol.Rules{AlwaysPublicFolders: []string{"/"}, PrivateTag: "private"}
	if d, _ := EvaluateSource(r, "a/b.md", nil); !d.Published {
		t.Error("root folder should publish everything")
	}
	r = protocol.Rules{AlwaysPublicFolders: []string{"/"}, ExcludeFolders: []string{"/"}}
	if d, _ := EvaluateSource(r, "a.md", nil); d.Published {
		t.Error("exclusion wins")
	}
}
