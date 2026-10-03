// Package source defines where the build pipeline reads content from
// (§2.2). The pipeline never talks HTTP; it reads immutable snapshots.
package source

import (
	"context"
	"encoding/json"
	"io"

	"github.com/klppl/kvist/internal/protocol"
)

// Source provides snapshots of vault content.
type Source interface {
	// Snapshot returns the current committed state. It must be immutable:
	// later commits produce new snapshots, never mutate old ones.
	Snapshot(ctx context.Context) (Snapshot, error)
	// Changes delivers a value whenever a new snapshot may be available.
	Changes() <-chan struct{}
}

// File is a vault file: vault-relative NFC path with forward slashes, hash,
// size and (informational) modification time.
type File = protocol.File

// Snapshot is one immutable state of a source.
type Snapshot interface {
	Revision() string // opaque, monotonic per source
	Files() []File    // sorted by Path
	Open(f File) (io.ReadCloser, error)
	Hints() *Hints // client link hints (§3.6); nil if none
}

// Hints maps, per note, link targets as written to the vault path the
// client resolved them to. A nil target means "not published".
type Hints struct {
	Version int                           `json:"version"`
	Notes   map[string]map[string]*string `json:"notes"`
}

// ReadHints decodes the hints file of a snapshot, if present and valid.
// Invalid hints are ignored: they can only ever make links less resolved.
func ReadHints(files []File, open func(File) (io.ReadCloser, error)) *Hints {
	for _, f := range files {
		if f.Path != protocol.HintsPath {
			continue
		}
		r, err := open(f)
		if err != nil {
			return nil
		}
		defer r.Close()
		var h Hints
		if err := json.NewDecoder(io.LimitReader(r, 64<<20)).Decode(&h); err != nil || h.Version != 1 {
			return nil
		}
		return &h
	}
	return nil
}
