// Package push is the content source backed by the server's content store,
// filled by clients through the sync protocol.
package push

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/klppl/kvist/internal/source"
	"github.com/klppl/kvist/internal/store"
)

// Source reads revisions of one site.
type Source struct {
	site *store.Site

	once    sync.Once
	changes chan struct{}
}

// New returns the source for a site.
func New(site *store.Site) *Source {
	return &Source{site: site, changes: make(chan struct{}, 1)}
}

// ErrEmpty is returned when nothing has been committed yet.
var ErrEmpty = errors.New("nothing has been published yet")

// Snapshot returns the HEAD revision.
func (s *Source) Snapshot(ctx context.Context) (source.Snapshot, error) {
	head, err := s.site.Head()
	if err != nil {
		return nil, err
	}
	if head == "" {
		return nil, ErrEmpty
	}
	return s.SnapshotAt(head)
}

// SnapshotAt returns a specific retained revision.
func (s *Source) SnapshotAt(revision string) (source.Snapshot, error) {
	rev, err := s.site.Revision(revision)
	if err != nil {
		return nil, err
	}
	return &snapshot{site: s.site, rev: rev}, nil
}

// Changes signals when HEAD changes, including commits made by another
// process. It polls HEAD's modification time.
func (s *Source) Changes() <-chan struct{} {
	s.once.Do(func() {
		go func() {
			last := s.site.HeadModTime()
			for range time.Tick(time.Second) {
				if t := s.site.HeadModTime(); !t.Equal(last) {
					last = t
					select {
					case s.changes <- struct{}{}:
					default:
					}
				}
			}
		}()
	})
	return s.changes
}

type snapshot struct {
	site  *store.Site
	rev   *store.Revision
	once  sync.Once
	hints *source.Hints
}

func (s *snapshot) Revision() string     { return s.rev.ID }
func (s *snapshot) Files() []source.File { return s.rev.Files }
func (s *snapshot) Open(f source.File) (io.ReadCloser, error) {
	return s.site.OpenBlob(f.Hash)
}

func (s *snapshot) Hints() *source.Hints {
	s.once.Do(func() { s.hints = source.ReadHints(s.rev.Files, s.Open) })
	return s.hints
}
