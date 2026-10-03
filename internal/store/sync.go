package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/klppl/kvist/internal/protocol"
)

// Sync is an open sync session: a validated manifest waiting for its blobs.
type Sync struct {
	ID        string            `json:"id"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Head      string            `json:"head"` // HEAD when the session started (compare-and-swap)
	Manifest  protocol.Manifest `json:"manifest"`
}

var syncIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// NewSyncID returns a random session id.
func NewSyncID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Site) syncPath(id string) string {
	return filepath.Join(s.dir, "syncs", id+".json")
}

// SaveSync stores a session.
func (s *Site) SaveSync(sy *Sync) error {
	if !syncIDRe.MatchString(sy.ID) {
		return errors.New("invalid sync id")
	}
	return writeJSONAtomic(s.syncPath(sy.ID), sy)
}

// Sync reads a session. Expired sessions are reported as ErrNotFound.
func (s *Site) Sync(id string, now time.Time) (*Sync, error) {
	if !syncIDRe.MatchString(id) {
		return nil, ErrNotFound
	}
	var sy Sync
	err := readJSON(s.syncPath(id), &sy)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !now.Before(sy.ExpiresAt) {
		return nil, ErrNotFound
	}
	return &sy, nil
}

// DeleteSync removes a session.
func (s *Site) DeleteSync(id string) error {
	if !syncIDRe.MatchString(id) {
		return nil
	}
	err := os.Remove(s.syncPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// GCStats summarizes a garbage collection.
type GCStats struct {
	ExpiredSyncs int
	DeletedBlobs int
	KeptBlobs    int
}

// GC removes expired sessions and blobs referenced by neither a retained
// revision nor an open session. The caller must hold the site lock.
func (s *Site) GC(now time.Time) (GCStats, error) {
	var st GCStats
	live := map[string]bool{}

	ents, err := os.ReadDir(filepath.Join(s.dir, "syncs"))
	if err != nil {
		return st, err
	}
	for _, e := range ents {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !syncIDRe.MatchString(id) {
			continue
		}
		sy, err := s.Sync(id, now)
		if errors.Is(err, ErrNotFound) {
			if err := s.DeleteSync(id); err != nil {
				return st, err
			}
			st.ExpiredSyncs++
			continue
		}
		if err != nil {
			return st, err
		}
		for _, f := range sy.Manifest.Files {
			live[f.Hash] = true
		}
	}

	revs, err := s.Revisions()
	if err != nil {
		return st, err
	}
	for _, id := range revs {
		r, err := s.Revision(id)
		if err != nil {
			return st, err
		}
		for _, f := range r.Files {
			live[f.Hash] = true
		}
	}

	root := filepath.Join(s.dir, "blobs", "sha256")
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		name := strings.ReplaceAll(rel, string(filepath.Separator), "")
		if strings.HasPrefix(d.Name(), ".upload-") {
			// Abandoned partial upload (e.g. after a crash); writers remove
			// their own temp files, so only old ones are left behind.
			if fi, err := d.Info(); err == nil && now.Sub(fi.ModTime()) > time.Hour {
				return os.Remove(p)
			}
			return nil
		}
		if live[protocol.HashPrefix+name] {
			st.KeptBlobs++
			return nil
		}
		st.DeletedBlobs++
		return os.Remove(p)
	})
	return st, err
}
