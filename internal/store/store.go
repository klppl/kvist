// Package store is the on-disk content store of a kvist server: a
// content-addressed blob store, immutable revisions, a HEAD pointer and open
// sync sessions, per site (§3.5).
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klppl/kvist/internal/protocol"
)

// Errors.
var (
	ErrNotFound        = errors.New("not found")
	ErrHeadMoved       = errors.New("head moved")
	ErrHashMismatch    = errors.New("content does not match hash")
	ErrSizeMismatch    = errors.New("content does not match size")
	ErrInvalidRevision = errors.New("invalid revision id")
)

// Store is the root of the data directory.
type Store struct {
	dir   string
	mu    sync.Mutex
	sites map[string]*Site
}

// Open opens (and creates) a data directory.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "sites"), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, sites: map[string]*Site{}}, nil
}

// Dir returns the data directory.
func (s *Store) Dir() string { return s.dir }

// Site returns the store of one site, creating its directories.
func (s *Store) Site(id string) (*Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.sites[id]; ok {
		return st, nil
	}
	dir := filepath.Join(s.dir, "sites", id)
	for _, d := range []string{"blobs/sha256", "revisions", "syncs", "builds", "cache"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			return nil, err
		}
	}
	st := &Site{ID: id, dir: dir}
	s.sites[id] = st
	return st, nil
}

// Site is the store of one site.
type Site struct {
	ID  string
	dir string
	mu  sync.Mutex // serializes commits, GC and rollbacks within the process
}

// Dir returns the site directory.
func (s *Site) Dir() string { return s.dir }

// Revision is an immutable committed manifest.
type Revision struct {
	ID          string          `json:"id"`
	Parent      string          `json:"parent,omitempty"`
	CommittedAt time.Time       `json:"committed_at"`
	Client      protocol.Client `json:"client"`
	Message     string          `json:"message,omitempty"`
	Files       []protocol.File `json:"files"`
}

// File returns the entry for path p, if present.
func (r *Revision) File(p string) (protocol.File, bool) {
	i := sort.Search(len(r.Files), func(i int) bool { return r.Files[i].Path >= p })
	if i < len(r.Files) && r.Files[i].Path == p {
		return r.Files[i], true
	}
	return protocol.File{}, false
}

// FormatRevision returns the id of revision number n.
func FormatRevision(n int) string { return fmt.Sprintf("r%06d", n) }

// ParseRevision returns the number of a revision id.
func ParseRevision(id string) (int, error) {
	if len(id) < 2 || id[0] != 'r' {
		return 0, ErrInvalidRevision
	}
	n, err := strconv.Atoi(id[1:])
	if err != nil || n < 1 {
		return 0, ErrInvalidRevision
	}
	return n, nil
}

// Lock takes the site lock (in-process and cross-process). Callers that
// commit, roll back or collect garbage hold it.
func (s *Site) Lock() (unlock func(), err error) {
	s.mu.Lock()
	fl, err := lockFile(filepath.Join(s.dir, ".lock"))
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	return func() { fl.unlock(); s.mu.Unlock() }, nil
}

// Head returns the current revision id, or "" if nothing was committed.
func (s *Site) Head() (string, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, "HEAD"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(b))
	if _, err := ParseRevision(id); err != nil {
		return "", fmt.Errorf("corrupt HEAD %q", id)
	}
	return id, nil
}

// HeadModTime returns the modification time of HEAD (zero if absent). It is
// used to notice commits made by another process.
func (s *Site) HeadModTime() time.Time {
	fi, err := os.Stat(filepath.Join(s.dir, "HEAD"))
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// Revision reads a committed revision.
func (s *Site) Revision(id string) (*Revision, error) {
	if _, err := ParseRevision(id); err != nil {
		return nil, err
	}
	var r Revision
	err := readJSON(filepath.Join(s.dir, "revisions", id+".json"), &r)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return &r, err
}

// HeadRevision reads the HEAD revision; nil if nothing was committed.
func (s *Site) HeadRevision() (*Revision, error) {
	id, err := s.Head()
	if err != nil || id == "" {
		return nil, err
	}
	return s.Revision(id)
}

// Revisions lists retained revision ids, oldest first.
func (s *Site) Revisions() ([]string, error) {
	ents, err := os.ReadDir(filepath.Join(s.dir, "revisions"))
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range ents {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if _, err := ParseRevision(id); ok && err == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids) // fixed-width numbers sort lexically
	return ids, nil
}

// CommitParams describe a new revision.
type CommitParams struct {
	ExpectedHead string // compare-and-swap: commit only if HEAD is still this
	Client       protocol.Client
	Message      string
	Files        []protocol.File
	Now          time.Time
	KeepRevs     int // retention; <1 keeps everything
}

// Commit atomically writes revision HEAD+1 and moves HEAD. The caller must
// hold the site lock. It returns ErrHeadMoved if HEAD is not ExpectedHead.
func (s *Site) Commit(p CommitParams) (*Revision, error) {
	head, err := s.Head()
	if err != nil {
		return nil, err
	}
	if head != p.ExpectedHead {
		return nil, ErrHeadMoved
	}
	n := 1
	if head != "" {
		h, _ := ParseRevision(head)
		n = h + 1
	}
	files := append([]protocol.File(nil), p.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		if !s.HasBlob(f.Hash) {
			return nil, fmt.Errorf("blob %s for %s is missing", f.Hash, f.Path)
		}
	}
	rev := &Revision{
		ID:          FormatRevision(n),
		Parent:      head,
		CommittedAt: p.Now.UTC(),
		Client:      p.Client,
		Message:     p.Message,
		Files:       files,
	}
	if err := writeJSONAtomic(filepath.Join(s.dir, "revisions", rev.ID+".json"), rev); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(filepath.Join(s.dir, "HEAD"), []byte(rev.ID+"\n"), 0o600); err != nil {
		return nil, err
	}
	if p.Client.ID != "" {
		if err := s.RecordClientPush(p.Client, rev.ID, rev.CommittedAt); err != nil {
			return nil, err
		}
	}
	if p.KeepRevs > 0 {
		if err := s.pruneRevisions(p.KeepRevs); err != nil {
			return nil, err
		}
	}
	return rev, nil
}

func (s *Site) pruneRevisions(keep int) error {
	ids, err := s.Revisions()
	if err != nil {
		return err
	}
	for len(ids) > keep {
		if err := os.Remove(filepath.Join(s.dir, "revisions", ids[0]+".json")); err != nil {
			return err
		}
		ids = ids[1:]
	}
	return nil
}

// ClientState records the last successful push of a client.
type ClientState struct {
	Name         string    `json:"name,omitempty"`
	LastPush     time.Time `json:"last_push"`
	LastRevision string    `json:"last_revision"`
}

func (s *Site) clientsPath() string { return filepath.Join(s.dir, "clients.json") }

// Clients returns the per-client push records.
func (s *Site) Clients() (map[string]ClientState, error) {
	m := map[string]ClientState{}
	err := readJSON(s.clientsPath(), &m)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	return m, err
}

// RecordClientPush notes that client c was in sync with revision at time at.
// The caller must hold the site lock.
func (s *Site) RecordClientPush(c protocol.Client, revision string, at time.Time) error {
	m, err := s.Clients()
	if err != nil {
		return err
	}
	m[c.ID] = ClientState{Name: c.Name, LastPush: at.UTC(), LastRevision: revision}
	return writeJSONAtomic(s.clientsPath(), m)
}

// --- blobs ---

func (s *Site) blobPath(h string) string {
	hexPart := strings.TrimPrefix(h, protocol.HashPrefix)
	return filepath.Join(s.dir, "blobs", "sha256", hexPart[:2], hexPart[2:])
}

// HasBlob reports whether a blob is stored.
func (s *Site) HasBlob(h string) bool {
	if !protocol.ValidHash(h) {
		return false
	}
	_, err := os.Stat(s.blobPath(h))
	return err == nil
}

// OpenBlob opens a stored blob.
func (s *Site) OpenBlob(h string) (*os.File, error) {
	if !protocol.ValidHash(h) {
		return nil, ErrNotFound
	}
	f, err := os.Open(s.blobPath(h))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

// PutBlob streams r into the blob store, verifying that it has exactly size
// bytes and hashes to h. A blob is either complete or absent; storing an
// existing blob again is a no-op that still verifies the upload.
func (s *Site) PutBlob(h string, size int64, r io.Reader) error {
	if !protocol.ValidHash(h) {
		return fmt.Errorf("invalid hash %q", h)
	}
	dst := s.blobPath(h)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(dst), ".upload-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	hw := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hw), io.LimitReader(r, size+1))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n != size {
		return ErrSizeMismatch
	}
	if sumHex(hw) != strings.TrimPrefix(h, protocol.HashPrefix) {
		return ErrHashMismatch
	}
	if err := os.Chmod(tmp, 0o400); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func sumHex(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }
