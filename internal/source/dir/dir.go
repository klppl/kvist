// Package dir is a read-only content source over a vault folder. It is for
// `kvist dev` and tests only: files are read when opened, so a snapshot is
// not strictly immutable, and there is no plugin in front of it, so the
// build's publish filter is the only gate.
package dir

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/source"
)

// Source reads a vault folder.
type Source struct {
	root     string
	interval time.Duration
	changes  chan struct{}
}

// New returns a source over root. Changes polls every interval (default 1s)
// once it is first called.
func New(root string) *Source {
	return &Source{root: root, interval: time.Second}
}

// Snapshot walks the folder and hashes every file. Dot folders (except the
// reserved .kvist files) and symlinks are skipped.
func (s *Source) Snapshot(ctx context.Context) (source.Snapshot, error) {
	var files []source.File
	abs := map[string]string{}
	var newest time.Time
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil || rel == "." {
			return err
		}
		vp := protocol.NormalizePath(filepath.ToSlash(rel))
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && vp != ".kvist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if strings.HasPrefix(vp, ".") && !protocol.IsReservedPath(vp) || vp == protocol.SiteNotePath {
			return nil // .kvist/site.md is generated below from the settings note
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := hashFile(p)
		if err != nil {
			return err
		}
		mt := info.ModTime().UTC().Truncate(time.Millisecond)
		if mt.After(newest) {
			newest = mt
		}
		files = append(files, source.File{Path: vp, Hash: h, Size: info.Size(), MTime: mt})
		abs[vp] = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The settings note (_site.md, anywhere) is read as .kvist/site.md, as
	// the plugin pushes it.
	var settings []source.File
	for _, f := range files {
		if protocol.IsSettingsNote(f.Path) {
			settings = append(settings, f)
		}
	}
	if len(settings) > 1 {
		names := make([]string, len(settings))
		for i, f := range settings {
			names[i] = f.Path
		}
		sort.Strings(names)
		return nil, fmt.Errorf("found %d settings notes (%s); keep one", len(names), strings.Join(names, ", "))
	}
	if len(settings) == 1 {
		f := settings[0]
		abs[protocol.SiteNotePath] = abs[f.Path]
		f.Path = protocol.SiteNotePath
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sig := sha256.New()
	for _, f := range files {
		io.WriteString(sig, f.Path+"\x00"+f.Hash+"\n")
	}
	return &snapshot{
		rev:   "dir-" + hex.EncodeToString(sig.Sum(nil))[:12],
		time:  newest,
		files: files,
		abs:   abs,
	}, nil
}

// Changes signals when the folder's file list or modification times change.
func (s *Source) Changes() <-chan struct{} {
	if s.changes != nil {
		return s.changes
	}
	s.changes = make(chan struct{}, 1)
	go func() {
		last := s.signature()
		for range time.Tick(s.interval) {
			if sig := s.signature(); sig != last {
				last = sig
				select {
				case s.changes <- struct{}{}:
				default:
				}
			}
		}
	}()
	return s.changes
}

func (s *Source) signature() string {
	h := sha256.New()
	_ = filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != ".kvist" && p != s.root {
			return filepath.SkipDir
		}
		if info, err := d.Info(); err == nil && !d.IsDir() {
			io.WriteString(h, p+info.ModTime().String()+"\n")
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return protocol.HashPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

type snapshot struct {
	rev   string
	time  time.Time
	files []source.File
	abs   map[string]string
}

func (s *snapshot) Revision() string     { return s.rev }
func (s *snapshot) Time() time.Time      { return s.time }
func (s *snapshot) Files() []source.File { return s.files }
func (s *snapshot) Hints() *source.Hints { return nil }

func (s *snapshot) Open(f source.File) (io.ReadCloser, error) {
	p, ok := s.abs[f.Path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return os.Open(p)
}
