package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIndexHTMLRedirects(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"index.html", "notes/index.html", "café/index.html"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte("<p>page</p>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := New([]Site{{Host: "example.com", PublicDir: dir}})
	for path, want := range map[string]string{
		"/index.html":                "/",
		"/notes/index.html":          "/notes/",
		"/notes/index.html?x=1":      "/notes/?x=1",
		"/caf%C3%A9/index.html":      "/caf%C3%A9/",
		"/notes/../notes/index.html": "/notes/",
	} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil))
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != want {
			t.Errorf("%s: %d %q, want 301 %q", path, rec.Code, rec.Header().Get("Location"), want)
		}
	}
	for path, code := range map[string]int{"/notes/": 200, "/": 200, "/missing/index.html": 404} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil))
		if rec.Code != code {
			t.Errorf("%s: %d, want %d", path, rec.Code, code)
		}
	}
}
