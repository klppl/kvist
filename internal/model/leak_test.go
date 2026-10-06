package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/source"
	"github.com/klppl/kvist/internal/source/dir"
)

const leakConfig = `
data_dir = "unused"
[[site]]
id = "garden"
base_url = "https://garden.example.com"
root_folder = "/" # keep the folder in addresses (root_test.go covers root folders)
  [site.publish]
  always_public_folders = ["Garden"]
  exclude_folders = ["Private", "Templates"]
  expose_frontmatter = ["stage"]
`

// hinted wraps a snapshot with client hints.
type hinted struct {
	source.Snapshot
	hints *source.Hints
}

func (h hinted) Hints() *source.Hints { return h.hints }

func str(s string) *string { return &s }

func buildFixture(t *testing.T, name string, hints *source.Hints) *Site {
	t.Helper()
	cfg, err := config.Parse([]byte(leakConfig))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := dir.New(filepath.Join("..", "..", "testdata", "leaks", name)).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var s source.Snapshot = snap
	if hints != nil {
		s = hinted{snap, hints}
	}
	site, err := Build(cfg.Sites[0], s)
	if err != nil {
		t.Fatal(err)
	}
	return site
}

// TestNoLeaks builds the leak fixture and asserts that no marker planted in
// private notes, private attachments or comments reaches the model: not in
// content, titles, paths, URLs, tags, backlinks, the graph, assets or params.
func TestNoLeaks(t *testing.T) {
	site := buildFixture(t, "basic", &source.Hints{Version: 1, Notes: map[string]map[string]*string{
		"Garden/HintUser.md": {
			"Collide": nil,                                    // the client saw a private "Collide"
			"Fake":    str("Private/LEAKMARK-path-secret.md"), // a lying hint: target not published
		},
	}})
	b, err := json.MarshalIndent(site, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	// Plain text and tag lists are not in the JSON; check them too.
	for _, n := range site.Notes {
		out += n.Text
	}
	for _, tag := range site.AllTags {
		out += tag.Name
	}
	if i := strings.Index(out, "LEAKMARK"); i >= 0 {
		lo, hi := max(0, i-200), min(len(out), i+100)
		t.Fatalf("private marker leaked into the model:\n…%s…", out[lo:hi])
	}
	for _, w := range site.Warnings {
		// Warnings go only to authenticated clients, but they must not quote
		// private content either (they may quote what the published note wrote).
		if strings.Contains(w.Message, "LEAKMARK-body") || strings.Contains(w.Message, "LEAKMARK-title") {
			t.Errorf("warning quotes private content: %+v", w)
		}
	}
}

func TestFixtureModel(t *testing.T) {
	site := buildFixture(t, "basic", &source.Hints{Version: 1, Notes: map[string]map[string]*string{
		"Garden/HintUser.md": {"Collide": nil},
	}})
	var paths []string
	for _, n := range site.Notes {
		paths = append(paths, n.Path)
	}
	want := []string{"Garden/Board.canvas", "Garden/Boards.md", "Garden/Collide.md", "Garden/Covered by path.md", "Garden/Covered.md", "Garden/Embedder.md", "Garden/HintUser.md", "Garden/Hub.md", "Garden/Leaf.md", "Journal/Linked.canvas"}
	sort.Strings(paths)
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("published = %v\nwant %v", paths, want)
	}
	if len(site.Assets) != 1 || site.Assets[0].Path != "attachments/public-pic.png" {
		t.Errorf("assets = %+v", site.Assets)
	}
	// Image properties pointing at private pictures are dropped; the
	// settings note's public default image is kept.
	for _, p := range []string{"Garden/Covered.md", "Garden/Covered by path.md"} {
		if img := site.Note(p).Image; img != "" {
			t.Errorf("%s: image = %q, want none", p, img)
		}
	}
	if site.Config.Image != site.Assets[0].URL {
		t.Errorf("site image = %q, want %q", site.Config.Image, site.Assets[0].URL)
	}
	if site.Config.Profile == nil || site.Config.Profile.Avatar != "" || site.Config.Profile.Bio != "A bio." {
		t.Errorf("profile = %+v", site.Config.Profile)
	}

	hub := site.Note("Garden/Hub.md")
	for _, frag := range []string{
		`<span class="link-unpublished">a private note</span>`,
		`<span class="link-unpublished">Shown Name</span>`,
		`<span class="link-unpublished">Missing note</span>`,
		`<span class="link-unpublished">markdown</span>`,
		`<a class="internal-link" href="/garden/leaf/">Leaf</a>`,
		`<img src="/_assets/`,
	} {
		if !strings.Contains(string(hub.Content), frag) {
			t.Errorf("hub lacks %q:\n%s", frag, hub.Content)
		}
	}
	if hub.Params["stage"] != "budding" || hub.Params["secret"] != nil {
		t.Errorf("params = %v", hub.Params)
	}
	// The missing link and the private link render identically.
	if strings.Count(string(hub.Content), `class="link-unpublished"`) != 8 {
		t.Errorf("expected 8 unpublished links:\n%s", hub.Content)
	}

	leaf := site.Note("Garden/Leaf.md")
	// From the hub and from the board's text and file cards (once).
	if len(leaf.Backlinks) != 2 || leaf.Backlinks[0].Source != site.Note("Garden/Board.canvas") || leaf.Backlinks[1].Source != hub {
		t.Errorf("leaf backlinks = %+v", leaf.Backlinks)
	}
	if len(hub.Backlinks) != 1 || hub.Backlinks[0].Source != leaf {
		t.Errorf("hub backlinks = %+v", hub.Backlinks)
	}
	if len(leaf.TagNames) != 1 || leaf.TagNames[0] != "garden/leaf" {
		t.Errorf("leaf tags = %v", leaf.TagNames)
	}
	if len(site.Tags) != 1 || site.Tags[0].Name != "garden" || len(site.Tags[0].Children) != 1 || len(site.Tags[0].Notes) != 1 {
		t.Errorf("tag tree = %+v", site.Tags)
	}

	// A null hint keeps [[Collide]] unresolved even though a public note of
	// that name exists.
	hu := site.Note("Garden/HintUser.md")
	if !strings.Contains(string(hu.Content), `<span class="link-unpublished">Collide</span>`) {
		t.Errorf("null hint ignored:\n%s", hu.Content)
	}

	// Embedder embeds itself (cycle) and a private note: both omitted.
	em := site.Note("Garden/Embedder.md")
	if strings.Contains(string(em.Content), "embed-content") {
		t.Errorf("embedder content:\n%s", em.Content)
	}
	// Hub embeds Embedder: one level of embed, recursion stops at the cycle.
	if strings.Count(string(hub.Content), `class="embed"`) != 1 {
		t.Errorf("hub embeds:\n%s", hub.Content)
	}

	linkers := map[string]bool{}
	for _, p := range []string{"Garden/Hub.md", "Garden/Leaf.md", "Garden/Embedder.md", "Garden/Boards.md", "Garden/Board.canvas"} {
		linkers[site.Note(p).ID] = true
	}
	for _, e := range site.Graph.Edges {
		if !linkers[e.Source] {
			t.Errorf("unexpected edge %+v", e)
		}
	}
	if len(site.Graph.Nodes) != len(site.Notes) {
		t.Errorf("graph nodes = %d", len(site.Graph.Nodes))
	}
}

// TestDeterministic builds twice and compares the JSON.
func TestDeterministic(t *testing.T) {
	a, _ := json.Marshal(buildFixture(t, "basic", nil))
	b, _ := json.Marshal(buildFixture(t, "basic", nil))
	if string(a) != string(b) {
		t.Error("two builds of the same snapshot differ")
	}
}

func TestURLCollision(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "Garden"), 0o755)
	os.WriteFile(filepath.Join(d, "Garden", "My Note.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(d, "Garden", "my-note.md"), []byte("b"), 0o644)
	cfg, _ := config.Parse([]byte(leakConfig))
	snap, _ := dir.New(d).Snapshot(context.Background())
	_, err := Build(cfg.Sites[0], snap)
	if err == nil || !strings.Contains(err.Error(), "/garden/my-note/") {
		t.Errorf("expected a URL collision error, got %v", err)
	}
}

// TestURLParity runs the URL fixtures shared with the plugin
// (plugin/test/parity.test.ts), which computes "open published page" URLs.
func TestURLParity(t *testing.T) {
	b, err := os.ReadFile("../../testdata/parity/urls.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Path, Permalink, Root, URL string }
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		bl := &builder{root: c.Root}
		if got := bl.noteURL(protocol.NormalizePath(c.Path), c.Permalink); got != c.URL {
			t.Errorf("%q (permalink %q, root %q): %q, want %q", c.Path, c.Permalink, c.Root, got, c.URL)
		}
	}
}
