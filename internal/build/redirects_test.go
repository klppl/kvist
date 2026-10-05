package build

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/serve"
	"github.com/klppl/kvist/internal/source/dir"
)

func TestHistoryNext(t *testing.T) {
	h := &History{
		Pages: map[string]Page{
			"Garden/Renamed.md":   {URL: "/garden/renamed/", Hash: "h1"},
			"Garden/Moved.md":     {URL: "/garden/moved/", Hash: "h2"},
			"Garden/Edited.md":    {URL: "/garden/edited/", Hash: "h3"},
			"Garden/Gone.md":      {URL: "/garden/gone/", Hash: "h4"},
			"Garden/Permalink.md": {URL: "/old-permalink/", Hash: "h5"},
			"Garden/Same.md":      {URL: "/garden/same/", Hash: "h6"},
			"Garden/Twin.md":      {URL: "/garden/twin/", Hash: "h7"},
		},
		Redirects: map[string]string{
			"/ancient/":        "/garden/moved/", // follows the note's next move
			"/stale/":          "/garden/gone/",  // its target is gone
			"/garden/same/x/":  "/garden/same/",  // still fine
			"/garden/revived/": "/garden/same/",  // its address has a page again
		},
	}
	pages := map[string]Page{
		"Garden/New name.md":      {URL: "/garden/new-name/", Hash: "h1"},
		"Elsewhere/Moved.md":      {URL: "/elsewhere/moved/", Hash: "h2"},
		"Elsewhere/Edited.md":     {URL: "/elsewhere/edited/", Hash: "changed"},
		"Garden/Permalink.md":     {URL: "/new-permalink/", Hash: "h5"},
		"Garden/Same.md":          {URL: "/garden/same/", Hash: "h6"},
		"A/Twin.md":               {URL: "/a/twin/", Hash: "x1"}, // two candidates by name: no guess
		"B/Twin.md":               {URL: "/b/twin/", Hash: "x2"},
		"Garden/Revived.md":       {URL: "/garden/revived/", Hash: "h8"},
		"Garden/Unrelated new.md": {URL: "/garden/unrelated-new/", Hash: "h9"},
	}
	live := map[string]bool{}
	for _, p := range pages {
		live[p.URL] = true
	}
	n := h.next(pages, func(u string) bool { return live[u] })
	want := map[string]string{
		"/garden/renamed/": "/garden/new-name/",
		"/garden/moved/":   "/elsewhere/moved/",
		"/garden/edited/":  "/elsewhere/edited/",
		"/old-permalink/":  "/new-permalink/",
		"/ancient/":        "/elsewhere/moved/",
		"/garden/same/x/":  "/garden/same/",
	}
	if !reflect.DeepEqual(n.Redirects, want) {
		t.Errorf("redirects =\n%v\nwant\n%v", n.Redirects, want)
	}
	if !reflect.DeepEqual(n.Pages, pages) {
		t.Error("pages not replaced")
	}
}

// TestRedirectsEndToEnd renames a note between two builds and checks the
// old address: a redirect page on disk and a 301 from the built-in server.
func TestRedirectsEndToEnd(t *testing.T) {
	cfg, err := config.Parse([]byte(fixtureConfig))
	if err != nil {
		t.Fatal(err)
	}
	sc := cfg.Sites[0]
	theme, err := LoadTheme("", sc)
	if err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	write := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(vault, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(vault, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	buildOnce := func(out, prev string, h *History) *History {
		t.Helper()
		snap, err := dir.New(vault).Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, h, err = WriteSiteIncremental(context.Background(), sc, theme, snap, out, prev, h)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	write("Garden/Old name.md", "# Moving\nSome text.")
	write("Garden/Stays.md", "# Stays")
	base := t.TempDir()
	h, err := LoadHistory(base)
	if err != nil {
		t.Fatal(err)
	}
	h = buildOnce(filepath.Join(base, "b1"), "", h)
	if len(h.Redirects) != 0 {
		t.Fatalf("first build redirects: %v", h.Redirects)
	}
	if err := h.Save(base); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(filepath.Join(vault, "Garden/Old name.md"), filepath.Join(vault, "Garden/New name.md")); err != nil {
		t.Fatal(err)
	}
	h, err = LoadHistory(base)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(base, "b2")
	h = buildOnce(out, filepath.Join(base, "b1"), h)
	if got := h.Redirects["/garden/old-name/"]; got != "/garden/new-name/" {
		t.Fatalf("redirects = %v", h.Redirects)
	}
	page, err := os.ReadFile(filepath.Join(out, "garden/old-name/index.html"))
	if err != nil || !strings.Contains(string(page), `url=/garden/new-name/`) || !strings.Contains(string(page), "https://garden.example.com/garden/new-name/") {
		t.Errorf("redirect page: %s %v", page, err)
	}
	var m map[string]string
	b, _ := os.ReadFile(filepath.Join(out, RedirectsFile))
	if json.Unmarshal(b, &m) != nil || m["/garden/old-name/"] != "/garden/new-name/" {
		t.Errorf("%s: %s", RedirectsFile, b)
	}
	if strings.Contains(readFile(t, filepath.Join(out, "sitemap.xml")), "old-name") {
		t.Error("the old address is in the sitemap")
	}

	srv := serve.New([]serve.Site{{PublicDir: out}})
	for _, u := range []string{"/garden/old-name/", "/garden/old-name"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u, nil))
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/garden/new-name/" {
			t.Errorf("GET %s: %d %q", u, rec.Code, rec.Header().Get("Location"))
		}
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+RedirectsFile, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("the redirect map is served: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/garden/stays/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("an unmoved page: %d", rec.Code)
	}

	// Writing a note at the old address again replaces the redirect.
	write("Garden/Old name.md", "# Back")
	h = buildOnce(filepath.Join(base, "b3"), out, h)
	if _, ok := h.Redirects["/garden/old-name/"]; ok {
		t.Errorf("redirect kept over a page: %v", h.Redirects)
	}
	if !strings.Contains(readFile(t, filepath.Join(base, "b3", "garden/old-name/index.html")), "Back") {
		t.Error("the new note's page was not written")
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
