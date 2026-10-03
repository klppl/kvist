package devserver

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klppl/kvist/internal/config"
)

func TestDevServerRebuilds(t *testing.T) {
	cfg, err := config.Parse([]byte(`data_dir = "x"
[[site]]
id = "dev"
base_url = "http://localhost:1313"
  [site.publish]
  always_public_folders = ["Garden"]
`))
	if err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	write := func(p, s string) {
		os.MkdirAll(filepath.Join(vault, filepath.Dir(p)), 0o755)
		if err := os.WriteFile(filepath.Join(vault, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Garden/Note.md", "first version")

	builds := make(chan error, 10)
	s := &Server{Site: cfg.Sites[0], Vault: vault, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnBuild: func(err error) { builds <- err }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := s.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-builds; err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	get := func(p string) (int, string) {
		resp, err := srv.Client().Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	code, body := get("/garden/note/")
	if code != 200 || !strings.Contains(body, "first version") || !strings.Contains(body, "/_kvist/livereload") {
		t.Fatalf("first page: %d %.300s", code, body)
	}
	wait := func() error {
		select {
		case err := <-builds:
			return err
		case <-time.After(10 * time.Second):
			t.Fatal("no rebuild after a change")
			return nil
		}
	}

	write("Garden/Note.md", "second version")
	if err := wait(); err != nil {
		t.Fatal(err)
	}
	if _, body := get("/garden/note/"); !strings.Contains(body, "second version") {
		t.Errorf("not rebuilt: %.300s", body)
	}

	// A broken vault keeps the last good build online.
	write("Garden/note.md", "collides with Note.md")
	if err := wait(); err == nil {
		t.Fatal("expected a build error")
	}
	if code, body := get("/garden/note/"); code != 200 || !strings.Contains(body, "second version") {
		t.Errorf("last good build not served: %d", code)
	}
}
