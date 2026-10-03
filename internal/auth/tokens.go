// Package auth manages per-site push tokens (§3.7). Tokens are created and
// revoked only through the CLI; the server reads tokens.json and reloads it
// when it changes.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// TokenPrefix marks kvist tokens so they are recognizable in leaks/scanners.
const TokenPrefix = "kvist_"

// ScopePush allows syncing and reading build status for one site.
const ScopePush = "push"

// Token is a stored token. The secret itself is never stored.
type Token struct {
	ID      string    `json:"id"`
	Site    string    `json:"site"`
	Name    string    `json:"name"`
	Scope   string    `json:"scope"`
	Hash    string    `json:"hash"` // hex SHA-256 of the secret
	Created time.Time `json:"created"`
}

// File is tokens.json.
type File struct {
	path string

	mu      sync.Mutex
	modTime time.Time
	size    int64
	tokens  []Token
}

// Open returns the token file in the data directory.
func Open(dataDir string) *File {
	return &File{path: filepath.Join(dataDir, "tokens.json")}
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// load reads the file if it changed since the last read.
func (f *File) load() error {
	fi, err := os.Stat(f.path)
	if errors.Is(err, os.ErrNotExist) {
		f.tokens, f.modTime, f.size = nil, time.Time{}, 0
		return nil
	}
	if err != nil {
		return err
	}
	if fi.ModTime().Equal(f.modTime) && fi.Size() == f.size && f.tokens != nil {
		return nil
	}
	b, err := os.ReadFile(f.path)
	if err != nil {
		return err
	}
	var toks []Token
	if err := json.Unmarshal(b, &toks); err != nil {
		return fmt.Errorf("tokens.json: %w", err)
	}
	if toks == nil {
		toks = []Token{}
	}
	f.tokens, f.modTime, f.size = toks, fi.ModTime(), fi.Size()
	return nil
}

func (f *File) save() error {
	b, err := json.MarshalIndent(f.tokens, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".tokens-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		return err
	}
	f.modTime = time.Time{} // force reload
	return nil
}

// Create adds a token for site and returns the secret (shown once).
func (f *File) Create(site, name string, now time.Time) (Token, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return Token{}, "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, "", err
	}
	secret := TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	idb := make([]byte, 4)
	if _, err := rand.Read(idb); err != nil {
		return Token{}, "", err
	}
	t := Token{
		ID:      hex.EncodeToString(idb),
		Site:    site,
		Name:    name,
		Scope:   ScopePush,
		Hash:    hashSecret(secret),
		Created: now.UTC(),
	}
	f.tokens = append(f.tokens, t)
	if err := f.save(); err != nil {
		return Token{}, "", err
	}
	return t, secret, nil
}

// List returns all tokens sorted by site and creation time.
func (f *File) List() ([]Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return nil, err
	}
	out := append([]Token(nil), f.tokens...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Site != out[j].Site {
			return out[i].Site < out[j].Site
		}
		return out[i].Created.Before(out[j].Created)
	})
	return out, nil
}

// Revoke deletes a token by id (or unique id prefix).
func (f *File) Revoke(id string) (Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return Token{}, err
	}
	idx := -1
	for i, t := range f.tokens {
		if id != "" && strings.HasPrefix(t.ID, id) {
			if idx >= 0 {
				return Token{}, fmt.Errorf("token id %q is ambiguous", id)
			}
			idx = i
		}
	}
	if idx < 0 {
		return Token{}, fmt.Errorf("no token with id %q", id)
	}
	t := f.tokens[idx]
	f.tokens = append(f.tokens[:idx], f.tokens[idx+1:]...)
	return t, f.save()
}

// Authenticate returns the token matching secret if it grants scope on site.
func (f *File) Authenticate(secret, site, scope string) (Token, bool) {
	if !strings.HasPrefix(secret, TokenPrefix) {
		return Token{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return Token{}, false
	}
	h := hashSecret(secret)
	var match Token
	found := false
	for _, t := range f.tokens {
		// Compare every token to keep timing independent of position.
		if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(h)) == 1 {
			match, found = t, true
		}
	}
	if !found || match.Site != site || match.Scope != scope {
		return Token{}, false
	}
	return match, true
}
