package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klppl/kvist/internal/build"
	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/serve"
	"github.com/klppl/kvist/internal/store"
)

// tinyPNG is a valid 1×1 PNG (assets are parsed to strip metadata).
const tinyPNG = "\x89\x50\x4e\x47\x0d\x0a\x1a\x0a\x00\x00\x00\x0d\x49\x48\x44\x52\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90\x77\x53\xde\x00\x00\x00\x0c\x49\x44\x41\x54\x78\x9c\x63\xb0\xaf\xf5\x03\x00\x02\x09\x01\x0b\x21\xed\x84\x45\x00\x00\x00\x00\x49\x45\x4e\x44\xae\x42\x60\x82"

func realBuilder(cfg *config.Config, st *store.Store) build.Builder {
	return &build.SiteBuilder{Config: cfg, Store: st}
}

func get(t *testing.T, h http.Handler, url string) (int, string, http.Header) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	b, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(b), rec.Result().Header
}

// TestPushBuildServe: a push is built, published atomically and served; an
// unpublished note disappears from the site; a failed build keeps the old
// site online.
func TestPushBuildServe(t *testing.T) {
	e := newEnvWith(t, realBuilder)
	d := e.device("laptop")
	d.write("Garden/Hello World.md", "---\nstage: seedling\n---\nHello! See [[Second]] and [[Diary]].", time.Time{})
	d.write("Garden/Second.md", "Second note ![[pic.png]]", time.Time{})
	d.write("Garden/pic.png", tinyPNG, time.Time{})
	d.write("Journal/Diary.md", "SECRET diary", time.Time{})
	res := d.mustPush()
	if res.Build.State != protocol.BuildSucceeded {
		t.Fatalf("build: %+v", res.Build)
	}

	site := e.site()
	public := filepath.Join(site.Dir(), "public")
	target, err := os.Readlink(public)
	if err != nil || target != filepath.Join("builds", res.Build.ID) {
		t.Fatalf("public -> %q, %v", target, err)
	}
	h := serve.New([]serve.Site{{Host: "garden.example.com", PublicDir: public}})

	code, body, hdr := get(t, h, "http://garden.example.com/garden/hello-world/")
	if code != 200 || !strings.Contains(body, "Hello!") || !strings.Contains(body, `<span class="link-unpublished">Diary</span>`) {
		t.Fatalf("note page: %d\n%s", code, body)
	}
	if hdr.Get("Cache-Control") != "public, max-age=60" {
		t.Errorf("HTML cache control = %q", hdr.Get("Cache-Control"))
	}
	if code, _, hdr := get(t, h, "http://garden.example.com/garden/hello-world"); code != 301 || hdr.Get("Location") != "/garden/hello-world/" {
		t.Errorf("redirect: %d %q", code, hdr.Get("Location"))
	}
	if code, body, _ := get(t, h, "http://garden.example.com/journal/diary/"); code != 404 || !strings.Contains(body, "Nothing grows here") {
		t.Errorf("private note: %d", code)
	}
	if code, _, _ := get(t, h, "http://garden.example.com/../../tokens.json"); code != 404 {
		t.Errorf("traversal: %d", code)
	}
	assets, _ := filepath.Glob(filepath.Join(public, "_assets", "*", "pic.png"))
	if len(assets) != 1 {
		t.Fatalf("assets = %v", assets)
	}
	rel := strings.TrimPrefix(assets[0], public)
	if code, _, hdr := get(t, h, "http://garden.example.com"+filepath.ToSlash(rel)); code != 200 || !strings.Contains(hdr.Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %d %q", code, hdr.Get("Cache-Control"))
	}
	walk := func() string {
		var all strings.Builder
		filepath.Walk(filepath.Join(site.Dir(), "builds"), func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				b, _ := os.ReadFile(p)
				all.Write(b)
			}
			return nil
		})
		return all.String()
	}
	if strings.Contains(walk(), "SECRET") {
		t.Error("private content in a build")
	}

	// Unpublish the second note.
	d.write("Garden/Second.md", "---\npublish: false\n---\nnow private", time.Time{})
	res2 := d.mustPush()
	if code, _, _ := get(t, h, "http://garden.example.com/garden/second/"); code != 404 {
		t.Errorf("unpublished page still served: %d (build %+v)", code, res2.Build)
	}

	// A broken push (URL collision) fails the build; the site stays up.
	d.write("Garden/hello-world.md", "collides", time.Time{})
	res3, err := d.push(false)
	if err != nil {
		t.Fatal(err)
	}
	if res3.Build.State != protocol.BuildFailed || !strings.Contains(res3.Build.Error, "/garden/hello-world/") {
		t.Fatalf("expected a failed build, got %+v", res3.Build)
	}
	if code, _, _ := get(t, h, "http://garden.example.com/garden/hello-world/"); code != 200 {
		t.Errorf("site went down after a failed build: %d", code)
	}
	if target2, _ := os.Readlink(public); target2 != filepath.Join("builds", res2.Build.ID) {
		t.Errorf("public moved to %q after a failed build", target2)
	}
}
