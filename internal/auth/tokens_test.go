package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTokens(t *testing.T) {
	dir := t.TempDir()
	f := Open(dir)
	now := time.Now()
	tok, secret, err := f.Create("garden", "laptop", now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, TokenPrefix) || len(secret) < 40 {
		t.Errorf("secret = %q", secret)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "tokens.json"))
	if strings.Contains(string(b), secret) {
		t.Fatal("secret stored in clear")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "tokens.json")); fi.Mode().Perm() != 0o600 {
		t.Errorf("tokens.json mode = %v", fi.Mode().Perm())
	}
	if _, ok := f.Authenticate(secret, "garden", ScopePush); !ok {
		t.Error("valid token rejected")
	}
	if _, ok := f.Authenticate(secret, "other", ScopePush); ok {
		t.Error("token accepted for another site")
	}
	if _, ok := f.Authenticate(secret+"x", "garden", ScopePush); ok {
		t.Error("wrong secret accepted")
	}

	// A second process (the CLI) revokes the token; the server notices.
	other := Open(dir)
	if _, err := other.Revoke(tok.ID[:4]); err != nil {
		t.Fatal(err)
	}
	// Make sure the modification time differs on coarse file systems.
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(filepath.Join(dir, "tokens.json"), later, later)
	if _, ok := f.Authenticate(secret, "garden", ScopePush); ok {
		t.Error("revoked token still accepted")
	}
}

func TestRevokeSite(t *testing.T) {
	f := Open(t.TempDir())
	now := time.Now()
	for _, site := range []string{"garden", "work", "garden"} {
		if _, _, err := f.Create(site, "laptop", now); err != nil {
			t.Fatal(err)
		}
	}
	revoked, err := f.RevokeSite("garden")
	if err != nil || len(revoked) != 2 {
		t.Fatalf("RevokeSite = %d tokens, %v", len(revoked), err)
	}
	list, _ := f.List()
	if len(list) != 1 || list[0].Site != "work" {
		t.Errorf("left = %+v", list)
	}
	if revoked, err := f.RevokeSite("garden"); err != nil || len(revoked) != 0 {
		t.Errorf("second RevokeSite = %d tokens, %v", len(revoked), err)
	}
}
