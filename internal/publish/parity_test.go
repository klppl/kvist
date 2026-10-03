package publish

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/vault"
)

// TestParity runs the fixtures shared with the Obsidian plugin's tests
// (plugin/test/parity.test.ts): both gates must decide the same way.
func TestParity(t *testing.T) {
	b, err := os.ReadFile("../../testdata/parity/rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Rules protocol.Rules `json:"rules"`
		Cases []struct {
			Path        string         `json:"path"`
			Frontmatter map[string]any `json:"frontmatter"`
			Tags        []string       `json:"tags"`
			Published   bool           `json:"published"`
			Reason      string         `json:"reason"`
		} `json:"cases"`
		Attachments []struct {
			Path    string `json:"path"`
			Allowed bool   `json:"allowed"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	for _, c := range fx.Cases {
		m := &vault.Meta{Frontmatter: c.Frontmatter}
		if m.Frontmatter == nil {
			m.Frontmatter = map[string]any{}
		}
		for _, tag := range c.Tags {
			m.Tags = append(m.Tags, trimHash(tag))
		}
		d := Evaluate(fx.Rules, c.Path, m)
		if d.Published != c.Published || d.Reason != c.Reason {
			t.Errorf("%s %v %v: got %+v, want %v/%s", c.Path, c.Frontmatter, c.Tags, d, c.Published, c.Reason)
		}
	}
	for _, a := range fx.Attachments {
		if got := AttachmentAllowed(fx.Rules, a.Path); got != a.Allowed {
			t.Errorf("attachment %s: %v, want %v", a.Path, got, a.Allowed)
		}
	}
}

func trimHash(s string) string {
	if len(s) > 0 && s[0] == '#' {
		return s[1:]
	}
	return s
}
