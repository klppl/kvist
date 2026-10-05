package pushclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/publish"
	"github.com/klppl/kvist/internal/resolve"
	"github.com/klppl/kvist/internal/vault"
)

// Decision is the gate-1 verdict for one note, for reports.
type Decision struct {
	Path string
	publish.Decision
}

// Scan is the result of applying the publish rules to a vault folder.
type Scan struct {
	Files     []protocol.File // manifest entries, sorted by path
	Decisions []Decision      // every note, sorted by path
	// UnpublishedLinks lists links from published notes to notes that exist
	// but are not published (the leak report).
	UnpublishedLinks []LinkReport
	open             map[string]func() (io.ReadCloser, error)
}

// LinkReport is a link from a published note to an unpublished one. The
// target path stays on the client; it is never sent to the server.
type LinkReport struct {
	From, Target string
	Embed        bool
}

// Open returns the content of a manifest entry.
func (s *Scan) Open(p string) (io.ReadCloser, error) {
	f, ok := s.open[p]
	if !ok {
		return nil, fmt.Errorf("%s is not in the manifest", p)
	}
	return f()
}

type hints struct {
	Version int                           `json:"version"`
	Notes   map[string]map[string]*string `json:"notes"`
}

// ScanDir walks a vault folder and builds the manifest under rules: the
// published notes, the attachments they reference, the optional settings
// note (_site.md, sent as .kvist/site.md) and .kvist/site.toml, and a
// .kvist/links.json hints file.
//
// Dot folders (.obsidian, .git, .trash) and symlinks are skipped.
func ScanDir(dir string, rules protocol.Rules) (*Scan, error) {
	type entry struct {
		abs  string
		info fs.FileInfo
	}
	all := map[string]entry{}
	folded := map[string]string{}
	err := filepath.WalkDir(dir, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, abs)
		if err != nil || rel == "." {
			return err
		}
		p := protocol.NormalizePath(filepath.ToSlash(rel))
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != ".kvist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // symlinks, devices
		}
		if strings.HasPrefix(p, ".kvist/") && !protocol.IsReservedPath(p) {
			return nil
		}
		if p == protocol.HintsPath || p == protocol.SiteNotePath {
			return nil // generated below
		}
		if other, dup := folded[strings.ToLower(p)]; dup {
			return fmt.Errorf("%q and %q differ only by case; rename one", other, p)
		}
		folded[strings.ToLower(p)] = p
		info, err := d.Info()
		if err != nil {
			return err
		}
		all[p] = entry{abs, info}
		return nil
	})
	if err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(all))
	for p := range all {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	ix := resolve.NewIndex(paths)

	s := &Scan{open: map[string]func() (io.ReadCloser, error){}}
	published := map[string]bool{}
	metas := map[string]*vault.Meta{}
	for _, p := range paths {
		if !protocol.IsNote(p) {
			continue
		}
		src, err := os.ReadFile(all[p].abs)
		if err != nil {
			return nil, err
		}
		d, m := publish.EvaluateSource(rules, p, src)
		s.Decisions = append(s.Decisions, Decision{p, d})
		if d.Published {
			published[p] = true
			metas[p] = m
		}
	}

	include := map[string]bool{}
	h := hints{Version: 1, Notes: map[string]map[string]*string{}}
	for _, p := range paths {
		m := metas[p]
		if m == nil {
			continue
		}
		include[p] = true
		for _, l := range m.Links {
			target, ok := ix.Resolve(p, l)
			if !ok || l.Target == "" {
				continue
			}
			if protocol.IsNote(target) {
				var v *string
				if published[target] {
					t := target
					v = &t
				} else {
					s.UnpublishedLinks = append(s.UnpublishedLinks, LinkReport{From: p, Target: target, Embed: l.Embed})
				}
				if !l.Markdown {
					if h.Notes[p] == nil {
						h.Notes[p] = map[string]*string{}
					}
					h.Notes[p][l.Target] = v
				}
				continue
			}
			if protocol.AllowedPath(target, rules) && publish.AttachmentAllowed(rules, target) {
				include[target] = true
			}
		}
		includeImages(include, ix, rules, p, m.Frontmatter, vault.NoteImageKeys)
	}
	if _, ok := all[protocol.SiteConfigPath]; ok {
		include[protocol.SiteConfigPath] = true
	}
	settings, err := findSettingsNote(paths)
	if err != nil {
		return nil, err
	}
	if settings != "" {
		src, err := os.ReadFile(all[settings].abs)
		if err != nil {
			return nil, err
		}
		// The settings note's images resolve from the vault root, as on
		// the server, which only sees it as .kvist/site.md.
		fm := vault.ParseMeta(src).Frontmatter
		includeImages(include, ix, rules, protocol.SettingsNoteName, fm, vault.SiteImageKeys)
		includeImages(include, ix, rules, protocol.SettingsNoteName, fm, vault.SiteAvatarKeys)
		includeImages(include, ix, rules, protocol.SettingsNoteName, fm, vault.SiteIconKeys)
	}

	for p := range include {
		e := all[p]
		hash, err := hashFile(e.abs)
		if err != nil {
			return nil, err
		}
		abs := e.abs
		s.open[p] = func() (io.ReadCloser, error) { return os.Open(abs) }
		s.Files = append(s.Files, protocol.File{
			Path:  p,
			Hash:  hash,
			Size:  e.info.Size(),
			MTime: e.info.ModTime().UTC().Truncate(time.Millisecond),
		})
	}
	if settings != "" {
		// The settings note goes up under a fixed name, wherever it lives.
		e := all[settings]
		hash, err := hashFile(e.abs)
		if err != nil {
			return nil, err
		}
		abs := e.abs
		s.open[protocol.SiteNotePath] = func() (io.ReadCloser, error) { return os.Open(abs) }
		s.Files = append(s.Files, protocol.File{
			Path:  protocol.SiteNotePath,
			Hash:  hash,
			Size:  e.info.Size(),
			MTime: e.info.ModTime().UTC().Truncate(time.Millisecond),
		})
	}
	if len(h.Notes) > 0 {
		b, err := json.Marshal(h) // map keys are sorted: deterministic
		if err != nil {
			return nil, err
		}
		s.open[protocol.HintsPath] = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }
		s.Files = append(s.Files, protocol.File{
			Path: protocol.HintsPath,
			Hash: protocol.HashBytes(b),
			Size: int64(len(b)),
			// The hints file changes whenever links change; give it the
			// newest mtime of its sources so the stale check treats it like
			// the notes it describes.
			MTime: newestMTime(s.Files),
		})
	}
	sort.Slice(s.Files, func(i, j int) bool { return s.Files[i].Path < s.Files[j].Path })
	return s, nil
}

// includeImages adds the vault image an image property points to (see
// vault.NoteImageKeys), under the same rules as an embedded attachment.
func includeImages(include map[string]bool, ix *resolve.Index, rules protocol.Rules, from string, fm map[string]any, keys []string) {
	ref, ok := vault.ImageProperty(fm, keys)
	if !ok || ref.URL != "" {
		return
	}
	target, ok := ix.Resolve(from, ref.Link)
	if ok && vault.IsImage(target) && protocol.AllowedPath(target, rules) && publish.AttachmentAllowed(rules, target) {
		include[target] = true
	}
}

func newestMTime(files []protocol.File) time.Time {
	var t time.Time
	for _, f := range files {
		if f.MTime.After(t) {
			t = f.MTime
		}
	}
	return t
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

// findSettingsNote returns the vault's settings note (_site.md in any
// folder), "" if there is none, or an error if there are several.
func findSettingsNote(paths []string) (string, error) {
	var found []string
	for _, p := range paths {
		if protocol.IsSettingsNote(p) {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("found %d settings notes (%s); keep one", len(found), strings.Join(found, ", "))
}
