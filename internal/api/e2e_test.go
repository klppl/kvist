package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klppl/kvist/internal/api"
	"github.com/klppl/kvist/internal/auth"
	"github.com/klppl/kvist/internal/build"
	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/pushclient"
	"github.com/klppl/kvist/internal/store"
	"github.com/klppl/kvist/internal/syncer"
)

const testConfig = `
data_dir = "unused"
sync_ttl = "1h"

[[site]]
id = "garden"
base_url = "https://garden.example.com"
root_folder = "/" # keep the folder in addresses (root_test.go covers root folders)

  [site.publish]
  always_public_folders = ["Garden"]
  exclude_folders = ["Private"]

[[site]]
id = "other"
base_url = "https://other.example.com"
`

type env struct {
	t      *testing.T
	cfg    *config.Config
	srv    *httptest.Server
	store  *store.Store
	svc    *syncer.Service
	token  string
	other  string // token for site "other"
	fail   atomic.Bool
	builds atomic.Int32

	mu  sync.Mutex
	now time.Time
}

func newEnv(t *testing.T) *env { return newEnvWith(t, nil) }

// newEnvWith uses the builder from mk, or a stub that only counts builds.
func newEnvWith(t *testing.T, mk func(*config.Config, *store.Store) build.Builder) *env {
	t.Helper()
	cfg, err := config.Parse([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	cfg.DataDir = data
	st, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, store: st, now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var b build.Builder = build.BuilderFunc(func(ctx context.Context, site, rev, id string) ([]protocol.Warning, error) {
		e.builds.Add(1)
		if e.fail.Load() {
			return nil, errors.New("theme exploded")
		}
		return nil, nil
	})
	if mk != nil {
		b = mk(cfg, st)
	}
	e.cfg = cfg
	q := build.NewQueue(b, func(site string) string { return filepath.Join(data, "sites", site) }, log)
	t.Cleanup(q.Close)
	e.svc = syncer.New(cfg, st, q, log)
	e.svc.Now = e.clock
	toks := auth.Open(data)
	if _, e.token, err = toks.Create("garden", "test", e.now); err != nil {
		t.Fatal(err)
	}
	if _, e.other, err = toks.Create("other", "test", e.now); err != nil {
		t.Fatal(err)
	}
	e.srv = httptest.NewServer(api.New(e.svc, toks, "test", log))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) clock() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.now
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func (e *env) client() *pushclient.Client {
	c, err := pushclient.New(e.srv.URL, "garden", e.token)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

func (e *env) site() *store.Site {
	s, err := e.store.Site("garden")
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *env) headPaths() []string {
	e.t.Helper()
	r, err := e.site().HeadRevision()
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	if r != nil {
		for _, f := range r.Files {
			out = append(out, f.Path)
		}
	}
	return out
}

// device is one vault + client state, like one phone or laptop.
type device struct {
	e     *env
	dir   string
	state string
	name  string
}

func (e *env) device(name string) *device {
	return &device{e: e, dir: e.t.TempDir(), state: filepath.Join(e.t.TempDir(), "state.json"), name: name}
}

func (d *device) write(p, content string, mtime time.Time) {
	d.e.t.Helper()
	abs := filepath.Join(d.dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		d.e.t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		d.e.t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(abs, mtime, mtime); err != nil {
			d.e.t.Fatal(err)
		}
	}
}

func (d *device) remove(p string) {
	if err := os.Remove(filepath.Join(d.dir, filepath.FromSlash(p))); err != nil {
		d.e.t.Fatal(err)
	}
}

func (d *device) rename(from, to string) {
	abs := filepath.Join(d.dir, filepath.FromSlash(to))
	os.MkdirAll(filepath.Dir(abs), 0o755)
	if err := os.Rename(filepath.Join(d.dir, filepath.FromSlash(from)), abs); err != nil {
		d.e.t.Fatal(err)
	}
}

func (d *device) push(force bool) (*pushclient.Result, error) {
	return pushclient.Push(context.Background(), d.e.client(), pushclient.Options{
		Dir: d.dir, StatePath: d.state, DeviceName: d.name, Force: force, Wait: true,
	})
}

func (d *device) mustPush() *pushclient.Result {
	d.e.t.Helper()
	res, err := d.push(false)
	if err != nil {
		d.e.t.Fatalf("push from %s: %v", d.name, err)
	}
	return res
}

func eq(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s:\n got %q\nwant %q", what, got, want)
	}
}

func TestPushPublishesOnlyPublicContent(t *testing.T) {
	e := newEnv(t)
	d := e.device("laptop")
	d.write("Garden/Welcome.md", "Hi! See [[Note]] and [[Secret Plans]] and ![[map.png]].", time.Time{})
	d.write("Garden/Note.md", "public by folder", time.Time{})
	d.write("Private/Note.md", "#public but excluded", time.Time{})
	d.write("Journal/Secret Plans.md", "SECRET-MARKER", time.Time{})
	d.write("Tagged.md", "#public ![](att/doc.pdf) ![](Private/photo.png)", time.Time{})
	d.write("Private/photo.png", "PNG-PRIVATE", time.Time{})
	d.write("att/doc.pdf", "%PDF", time.Time{})
	d.write("map.png", "PNG", time.Time{})
	d.write("unused.png", "PNG", time.Time{})
	d.write("tool.exe", "MZ", time.Time{})
	d.write(".obsidian/app.json", "{}", time.Time{})
	d.write(".kvist/site.toml", "title = 'x'", time.Time{})
	d.write(".kvist/secret.txt", "no", time.Time{})
	d.write("Garden/_site.md", "---\ntitle: Note title\n---\n- [About](/about/)\n", time.Time{})

	res := d.mustPush()
	// The settings note travels as .kvist/site.md and is never a page.
	eq(t, "published", e.headPaths(), []string{
		".kvist/links.json", ".kvist/site.md", "Garden/Note.md", "Garden/Welcome.md", "Tagged.md", "att/doc.pdf", "map.png",
	})
	if res.Revision != "r000001" || res.Build == nil || res.Build.State != protocol.BuildSucceeded {
		t.Fatalf("result = %+v build=%+v", res, res.Build)
	}
	if len(res.Scan.UnpublishedLinks) != 1 || res.Scan.UnpublishedLinks[0].Target != "Journal/Secret Plans.md" {
		t.Errorf("leak report = %+v", res.Scan.UnpublishedLinks)
	}

	// The hints file maps the private target to null and never names it.
	head, _ := e.site().HeadRevision()
	f, _ := head.File(protocol.HintsPath)
	blob, _ := e.site().OpenBlob(f.Hash)
	b, _ := io.ReadAll(blob)
	blob.Close()
	if bytes.Contains(b, []byte("Journal")) {
		t.Errorf("hints leak a private path: %s", b)
	}
	var h struct {
		Notes map[string]map[string]*string `json:"notes"`
	}
	json.Unmarshal(b, &h)
	if v, ok := h.Notes["Garden/Welcome.md"]["Secret Plans"]; !ok || v != nil {
		t.Errorf("hint for private target = %v, %v", v, ok)
	}
	if v := h.Notes["Garden/Welcome.md"]["Note"]; v == nil || *v != "Garden/Note.md" {
		t.Errorf("hint for public target = %v", v)
	}
	// No blob of a private note reached the server.
	if e.site().HasBlob(protocol.HashBytes([]byte("SECRET-MARKER"))) {
		t.Error("private note was uploaded")
	}
}

func TestUnchangedPushIsANoop(t *testing.T) {
	e := newEnv(t)
	d := e.device("laptop")
	d.write("Garden/A.md", "a", time.Time{})
	d.mustPush()
	builds := e.builds.Load()
	res := d.mustPush()
	if !res.Unchanged || res.Revision != "r000001" || res.BuildID != "" {
		t.Errorf("second push = %+v", res)
	}
	if e.builds.Load() != builds {
		t.Error("an unchanged push started a build")
	}
	if revs, _ := e.site().Revisions(); len(revs) != 1 {
		t.Errorf("revisions = %v", revs)
	}
}

// §3.2: unpublish.
func TestUnpublishRemovesNoteAndItsAttachments(t *testing.T) {
	e := newEnv(t)
	d := e.device("laptop")
	d.write("Tagged.md", "#public ![[pic.png]]", time.Time{})
	d.write("pic.png", "PNG", time.Time{})
	d.mustPush()
	eq(t, "before", e.headPaths(), []string{"Tagged.md", "pic.png"})

	d.write("Tagged.md", "no longer public ![[pic.png]]", time.Time{})
	d.mustPush()
	eq(t, "after", e.headPaths(), nil)
}

// §3.2: rename / move uploads nothing.
func TestRenameUploadsNothing(t *testing.T) {
	e := newEnv(t)
	d := e.device("laptop")
	d.write("Garden/A.md", "content a", time.Time{})
	d.write("Garden/img.png", "PNG", time.Time{})
	d.write("Garden/B.md", "![[img.png]]", time.Time{})
	if res := d.mustPush(); res.Uploaded != 3 {
		t.Fatalf("first push uploaded %d", res.Uploaded)
	}
	d.rename("Garden/A.md", "Garden/Sub/Renamed A.md")
	res := d.mustPush()
	if res.Uploaded != 0 {
		t.Errorf("rename uploaded %d blobs", res.Uploaded)
	}
	eq(t, "after rename", e.headPaths(), []string{"Garden/B.md", "Garden/Sub/Renamed A.md", "Garden/img.png"})
}

// §3.2: a missed push is healed by the next one, because every manifest is
// the complete state.
func TestMissedPushHeals(t *testing.T) {
	e := newEnv(t)
	d := e.device("laptop")
	d.write("Garden/A.md", "v1", time.Time{})
	d.mustPush()
	// Changes that never got pushed (offline, crash, …).
	d.write("Garden/A.md", "v2", time.Time{})
	d.write("Garden/B.md", "new", time.Time{})
	// More changes, then a successful push.
	d.remove("Garden/B.md")
	d.write("Garden/C.md", "c", time.Time{})
	d.write("Garden/A.md", "v3", time.Time{})
	d.mustPush()
	eq(t, "healed", e.headPaths(), []string{"Garden/A.md", "Garden/C.md"})
	head, _ := e.site().HeadRevision()
	if f, _ := head.File("Garden/A.md"); f.Hash != protocol.HashBytes([]byte("v3")) {
		t.Error("A.md is not the latest version")
	}
}

func manifestFor(t *testing.T, c *pushclient.Client, base string, files map[string]string) (protocol.Manifest, map[string][]byte) {
	t.Helper()
	si, err := c.SiteInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := protocol.Manifest{BaseRevision: base, RulesHash: si.RulesHash, Client: protocol.Client{ID: "manual", Name: "manual"}}
	blobs := map[string][]byte{}
	for p, content := range files {
		h := protocol.HashBytes([]byte(content))
		blobs[h] = []byte(content)
		m.Files = append(m.Files, protocol.File{Path: p, Hash: h, Size: int64(len(content)), MTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	}
	return m, blobs
}

// §3.2: interrupted upload — nothing becomes visible, a retry reuses the
// uploaded blobs, and expired sessions are garbage-collected.
func TestInterruptedUpload(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	m, blobs := manifestFor(t, c, "", map[string]string{"Garden/A.md": "aaa", "Garden/B.md": "bbb"})
	sy, err := c.StartSync(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(sy.Missing) != 2 {
		t.Fatalf("missing = %v", sy.Missing)
	}
	// Upload one blob, then "lose the connection".
	if err := c.PutBlob(ctx, sy.SyncID, sy.Missing[0], bytes.NewReader(blobs[sy.Missing[0]])); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Commit(ctx, sy.SyncID, false); !pushclient.IsCode(err, protocol.ErrMissingBlobs) {
		t.Fatalf("commit with missing blob: %v", err)
	}
	if h, _ := e.site().Head(); h != "" {
		t.Fatal("partial sync became visible")
	}

	// Retry: the already uploaded blob is not requested again.
	sy2, err := c.StartSync(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(sy2.Missing) != 1 || sy2.Missing[0] != sy.Missing[1] {
		t.Fatalf("retry missing = %v", sy2.Missing)
	}

	// Abandon everything; after the TTL, GC removes sessions and blobs.
	e.advance(2 * time.Hour)
	e.svc.GCAll()
	if e.site().HasBlob(sy.Missing[0]) {
		t.Error("orphan blob survived GC")
	}
	if _, err := c.Commit(ctx, sy2.SyncID, false); !pushclient.IsCode(err, protocol.ErrSyncExpired) {
		t.Errorf("commit on expired sync: %v", err)
	}
}

// §3.2: two devices pushing at once — compare-and-swap on the revision.
func TestConcurrentCommitsConflict(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	m1, b1 := manifestFor(t, c, "", map[string]string{"Garden/A.md": "one"})
	m2, b2 := manifestFor(t, c, "", map[string]string{"Garden/B.md": "two"})
	s1, err := c.StartSync(ctx, m1)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := c.StartSync(ctx, m2)
	if err != nil {
		t.Fatal(err)
	}
	for h, b := range b1 {
		c.PutBlob(ctx, s1.SyncID, h, bytes.NewReader(b))
	}
	for h, b := range b2 {
		c.PutBlob(ctx, s2.SyncID, h, bytes.NewReader(b))
	}
	if _, err := c.Commit(ctx, s1.SyncID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Commit(ctx, s2.SyncID, false); !pushclient.IsCode(err, protocol.ErrRevisionChanged) {
		t.Fatalf("second commit: %v", err)
	}
	eq(t, "winner", e.headPaths(), []string{"Garden/A.md"})
}

// §3.2: a failed build leaves the revision committed and reports failure.
func TestFailedBuild(t *testing.T) {
	e := newEnv(t)
	d := e.device("laptop")
	d.write("Garden/A.md", "a", time.Time{})
	e.fail.Store(true)
	res := d.mustPush()
	if res.Build.State != protocol.BuildFailed || !strings.Contains(res.Build.Error, "theme exploded") {
		t.Fatalf("build = %+v", res.Build)
	}
	if h, _ := e.site().Head(); h != res.Revision {
		t.Errorf("HEAD = %s, want %s", h, res.Revision)
	}
	st, err := e.client().BuildStatus(context.Background(), res.BuildID, 0)
	if err != nil || st.State != protocol.BuildFailed {
		t.Errorf("status = %+v, %v", st, err)
	}
}

// §3.3: a device with older content asks for confirmation; newer content
// from a device with a stale base goes through with a warning.
func TestStaleState(t *testing.T) {
	e := newEnv(t)
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t1.Add(time.Hour)

	a := e.device("laptop")
	a.write("Garden/Shared.md", "new version", t1)
	a.mustPush()

	// The phone hasn't synced yet and holds the old version.
	b := e.device("phone")
	b.write("Garden/Shared.md", "old version", t0)
	_, err := b.push(false)
	var se *pushclient.StaleError
	if !errors.As(err, &se) || len(se.Regressions) != 1 || se.Regressions[0].Kind != protocol.RegressionOlder {
		t.Fatalf("expected a stale-state error, got %v", err)
	}
	if !strings.Contains(se.Message, "1 note would revert") {
		t.Errorf("message = %q", se.Message)
	}
	head, _ := e.site().HeadRevision()
	if f, _ := head.File("Garden/Shared.md"); f.Hash != protocol.HashBytes([]byte("new version")) {
		t.Fatal("old content was committed without confirmation")
	}
	// Confirmed by the user.
	if _, err := b.push(true); err != nil {
		t.Fatal(err)
	}

	// The laptop's base is now stale, but it holds newer content: accepted.
	a.write("Garden/Shared.md", "newest version", t2)
	res := a.mustPush()
	found := false
	for _, w := range res.Warnings {
		found = found || w.Code == protocol.WarnStaleBase
	}
	if !found {
		t.Errorf("expected a stale_base warning, got %+v", res.Warnings)
	}
}

func TestStaleStateRemoval(t *testing.T) {
	e := newEnv(t)
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	a := e.device("laptop")
	b := e.device("phone")
	a.write("Garden/A.md", "a", t0)
	b.write("Garden/A.md", "a", t0)
	a.mustPush()
	b.mustPush() // stale base (new device), no regressions: same content

	e.advance(time.Hour)
	// The laptop writes a new note after the phone's last push.
	a.write("Garden/New.md", "fresh", e.clock().Add(time.Minute))
	a.mustPush()
	// The phone hasn't received it yet; its push would delete it.
	_, err := b.push(false)
	var se *pushclient.StaleError
	if !errors.As(err, &se) || len(se.Regressions) != 1 || se.Regressions[0].Kind != protocol.RegressionRemoved {
		t.Fatalf("expected removal regression, got %v", err)
	}
	// Deleting a note the phone itself had pushed before is fine.
	b.write("Garden/New.md", "fresh", e.clock().Add(time.Minute))
	b.mustPush()
	b.remove("Garden/New.md")
	if _, err := b.push(false); err != nil {
		t.Errorf("deleting own content: %v", err)
	}
}

// §5.2: gate 2 drops notes that are not public under the server's rules.
func TestGate2DropsPrivateNotes(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	m, blobs := manifestFor(t, c, "", map[string]string{
		"Garden/ok.md":       "fine",
		"Private/leak.md":    "#public",
		"untagged.md":        "no rule",
		"Garden/private.md":  "#private",
		"Garden/badfm.md":    "---\npublish: [\n---\n",
		"Garden/picture.png": "PNG",
		"Private/photo.png":  "PNG2",
	})
	sy, err := c.StartSync(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range sy.Missing {
		if err := c.PutBlob(ctx, sy.SyncID, h, bytes.NewReader(blobs[h])); err != nil {
			t.Fatal(err)
		}
	}
	cr, err := c.Commit(ctx, sy.SyncID, false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "committed", e.headPaths(), []string{"Garden/ok.md", "Garden/picture.png"})
	var dropped []string
	for _, w := range cr.Warnings {
		if w.Code == protocol.WarnGateDisagreement {
			dropped = append(dropped, w.Path)
		}
	}
	eq(t, "warnings", dropped, []string{"Garden/badfm.md", "Garden/private.md", "Private/leak.md", "Private/photo.png", "untagged.md"})
}

func TestManifestValidation(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	bad := []string{
		"../escape.md", "/abs.md", "a//b.md", "a/./b.md", "Cafe\u0301.md", // NFD
		".obsidian/workspace.md", "x/.hidden.md", "virus.exe", "ctrl\x01.md", ".kvist/other.json", ".kvist/site.toml",
	}
	for _, p := range bad {
		m, _ := manifestFor(t, c, "", map[string]string{p: "x"})
		if _, err := c.StartSync(ctx, m); !pushclient.IsCode(err, protocol.ErrInvalidManifest) {
			t.Errorf("%q: %v", p, err)
		}
	}
	m, _ := manifestFor(t, c, "", map[string]string{"Garden/A.md": "x", "garden/a.md": "y"})
	if _, err := c.StartSync(ctx, m); !pushclient.IsCode(err, protocol.ErrInvalidManifest) {
		t.Errorf("case collision: %v", err)
	}
	m, _ = manifestFor(t, c, "", map[string]string{"Garden/A.md": "x"})
	m.RulesHash = protocol.HashBytes([]byte("stale"))
	if _, err := c.StartSync(ctx, m); !pushclient.IsCode(err, protocol.ErrRulesChanged) {
		t.Errorf("stale rules: %v", err)
	}
	m, _ = manifestFor(t, c, "", map[string]string{"Garden/A.md": "x", ".kvist/site.md": "x", ".kvist/links.json": "{}", "Café.md": "nfc"})
	if _, err := c.StartSync(ctx, m); err != nil {
		t.Errorf("valid manifest rejected: %v", err)
	}
}

func TestBlobUploadChecks(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	m, _ := manifestFor(t, c, "", map[string]string{"Garden/A.md": "right"})
	sy, err := c.StartSync(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	h := sy.Missing[0]
	if err := c.PutBlob(ctx, sy.SyncID, h, strings.NewReader("wrong")); !pushclient.IsCode(err, protocol.ErrHashMismatch) {
		t.Errorf("wrong content: %v", err)
	}
	if err := c.PutBlob(ctx, sy.SyncID, h, strings.NewReader("right but longer")); !pushclient.IsCode(err, protocol.ErrHashMismatch) {
		t.Errorf("wrong size: %v", err)
	}
	if e.site().HasBlob(h) {
		t.Fatal("bad upload was stored")
	}
	other := protocol.HashBytes([]byte("not in manifest"))
	if err := c.PutBlob(ctx, sy.SyncID, other, strings.NewReader("not in manifest")); !pushclient.IsCode(err, protocol.ErrUnexpectedBlob) {
		t.Errorf("unexpected blob: %v", err)
	}
	if err := c.PutBlob(ctx, sy.SyncID, h, strings.NewReader("right")); err != nil {
		t.Fatal(err)
	}
	if err := c.PutBlob(ctx, sy.SyncID, h, strings.NewReader("right")); err != nil {
		t.Errorf("re-upload is not idempotent: %v", err)
	}
}

func TestAuthAndProtocolHeader(t *testing.T) {
	e := newEnv(t)
	get := func(path, token string, proto bool) (int, string) {
		req, _ := http.NewRequest("GET", e.srv.URL+path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if proto {
			req.Header.Set(protocol.HeaderProtocol, "1")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, _ := get("/api/v1/info", "", false); code != 200 {
		t.Errorf("info without header: %d", code)
	}
	if code, _ := get("/api/v1/sites/garden", e.token, true); code != 200 {
		t.Errorf("valid token: %d", code)
	}
	if code, _ := get("/api/v1/sites/garden", e.token, false); code != 400 {
		t.Errorf("missing protocol header: %d", code)
	}
	codeNone, bodyNone := get("/api/v1/sites/garden", "", true)
	codeWrong, bodyWrong := get("/api/v1/sites/garden", e.other, true)
	codeUnknown, bodyUnknown := get("/api/v1/sites/nope", e.token, true)
	codeBogus, bodyBogus := get("/api/v1/sites/garden", "kvist_bogus", true)
	for _, c := range []int{codeNone, codeWrong, codeUnknown, codeBogus} {
		if c != 401 {
			t.Errorf("expected 401, got %d", c)
		}
	}
	if bodyNone != bodyWrong || bodyWrong != bodyUnknown || bodyUnknown != bodyBogus {
		t.Errorf("unauthenticated errors differ:\n%s%s%s%s", bodyNone, bodyWrong, bodyUnknown, bodyBogus)
	}
	if _, err := pushclient.New("http://example.com", "garden", e.token); err == nil {
		t.Error("client must refuse plain http to non-localhost hosts")
	}
}

func TestRetentionAndRollback(t *testing.T) {
	e := newEnv(t)
	d := e.device("laptop")
	for i := 0; i < 12; i++ {
		d.write("Garden/A.md", strings.Repeat("v", i+1), time.Time{})
		d.mustPush()
	}
	revs, _ := e.site().Revisions()
	if len(revs) != config.DefaultRevisions || revs[len(revs)-1] != "r000012" {
		t.Fatalf("revisions = %v", revs)
	}
	e.svc.GCAll()
	if e.site().HasBlob(protocol.HashBytes([]byte("v"))) {
		t.Error("blob of a pruned revision survived GC")
	}
	old, err := e.site().Revision("r000005")
	if err != nil {
		t.Fatal(err)
	}
	unlock, _ := e.site().Lock()
	rev, err := e.site().Commit(store.CommitParams{ExpectedHead: "r000012", Files: old.Files, Now: e.clock(), Message: "rollback to r000005"})
	unlock()
	if err != nil || rev.ID != "r000013" {
		t.Fatalf("rollback: %v %+v", err, rev)
	}
}
