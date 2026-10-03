// Package syncer implements the server side of the sync protocol (§3):
// manifest validation, sessions, blob uploads, the stale-state check, gate 2
// at commit, and handing commits to the build queue. It is transport-neutral;
// package api maps it to HTTP.
package syncer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/klppl/kvist/internal/build"
	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/publish"
	"github.com/klppl/kvist/internal/store"
)

// Service is the sync service for all configured sites.
type Service struct {
	cfg   *config.Config
	store *store.Store
	queue *build.Queue
	log   *slog.Logger
	// Now is the clock; tests replace it.
	Now func() time.Time
}

// New returns a sync service.
func New(cfg *config.Config, st *store.Store, q *build.Queue, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{cfg: cfg, store: st, queue: q, log: log, Now: time.Now}
}

func (s *Service) site(id string) (*config.Site, *store.Site, error) {
	sc := s.cfg.Site(id)
	if sc == nil {
		return nil, nil, errUnknownSite()
	}
	ss, err := s.store.Site(id)
	if err != nil {
		return nil, nil, err
	}
	return sc, ss, nil
}

// SiteInfo answers GET /sites/{site}.
func (s *Service) SiteInfo(site string) (protocol.SiteInfo, error) {
	sc, ss, err := s.site(site)
	if err != nil {
		return protocol.SiteInfo{}, err
	}
	head, err := ss.Head()
	if err != nil {
		return protocol.SiteInfo{}, err
	}
	rules := sc.Rules()
	return protocol.SiteInfo{
		Site:      site,
		Rules:     rules,
		RulesHash: rules.Hash(),
		Limits:    sc.ProtocolLimits(),
		Revision:  head,
	}, nil
}

// maxClientField bounds client-supplied identity strings.
const maxClientField = 128

func validateManifest(m *protocol.Manifest, rules protocol.Rules, lim protocol.Limits) error {
	if m.RulesHash != rules.Hash() {
		return newError(http.StatusConflict, protocol.ErrRulesChanged, "publish rules changed; fetch site info and rebuild the manifest")
	}
	if m.Client.ID == "" || len(m.Client.ID) > maxClientField || len(m.Client.Name) > maxClientField ||
		len(m.Client.Platform) > maxClientField || len(m.Client.Version) > maxClientField {
		return newError(http.StatusBadRequest, protocol.ErrInvalidManifest, "client.id is required and client fields are limited to %d bytes", maxClientField)
	}
	if m.BaseRevision != "" {
		if _, err := store.ParseRevision(m.BaseRevision); err != nil {
			return newError(http.StatusBadRequest, protocol.ErrInvalidManifest, "invalid base_revision")
		}
	}
	if len(m.Files) > lim.MaxFiles {
		return newError(http.StatusRequestEntityTooLarge, protocol.ErrTooLarge, "manifest has %d files; the limit is %d", len(m.Files), lim.MaxFiles)
	}
	var bad []protocol.PathError
	addBad := func(p, msg string) { bad = append(bad, protocol.PathError{Path: p, Message: msg}) }
	seen := map[string]bool{}
	folded := map[string]string{}
	sizes := map[string]int64{}
	for _, f := range m.Files {
		if err := protocol.ValidatePath(f.Path, lim.MaxPathBytes); err != nil {
			addBad(f.Path, err.Error())
			continue
		}
		if !protocol.AllowedPath(f.Path, rules) {
			addBad(f.Path, "file type or location not allowed")
			continue
		}
		if seen[f.Path] {
			addBad(f.Path, "duplicate path")
			continue
		}
		seen[f.Path] = true
		k := strings.ToLower(f.Path)
		if other, ok := folded[k]; ok {
			addBad(f.Path, fmt.Sprintf("differs from %q only by case", other))
			continue
		}
		folded[k] = f.Path
		if !protocol.ValidHash(f.Hash) {
			addBad(f.Path, "invalid hash")
			continue
		}
		if f.Size < 0 || f.Size > lim.MaxFileSize {
			addBad(f.Path, fmt.Sprintf("size must be between 0 and %d bytes", lim.MaxFileSize))
			continue
		}
		if prev, ok := sizes[f.Hash]; ok && prev != f.Size {
			addBad(f.Path, "same hash listed with different sizes")
			continue
		}
		sizes[f.Hash] = f.Size
	}
	if len(bad) > 0 {
		return newError(http.StatusBadRequest, protocol.ErrInvalidManifest, "%d invalid manifest entries", len(bad)).withDetails(bad)
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	for i := range m.Files {
		m.Files[i].MTime = m.Files[i].MTime.UTC()
	}
	return nil
}

// StartSync answers POST /syncs.
func (s *Service) StartSync(site string, m protocol.Manifest) (protocol.SyncResponse, error) {
	sc, ss, err := s.site(site)
	if err != nil {
		return protocol.SyncResponse{}, err
	}
	if err := validateManifest(&m, sc.Rules(), sc.ProtocolLimits()); err != nil {
		return protocol.SyncResponse{}, err
	}
	head, err := ss.HeadRevision()
	if err != nil {
		return protocol.SyncResponse{}, err
	}
	now := s.Now().UTC()
	sy := &store.Sync{
		ID:        store.NewSyncID(),
		CreatedAt: now,
		ExpiresAt: now.Add(s.cfg.SyncTTL.Duration),
		Head:      revID(head),
		Manifest:  m,
	}
	if err := ss.SaveSync(sy); err != nil {
		return protocol.SyncResponse{}, err
	}
	check, err := s.baseCheck(ss, head, m)
	if err != nil {
		return protocol.SyncResponse{}, err
	}
	return protocol.SyncResponse{
		SyncID:    sy.ID,
		Missing:   missingHashes(ss, m.Files),
		ExpiresAt: sy.ExpiresAt,
		BaseCheck: check,
	}, nil
}

func revID(r *store.Revision) string {
	if r == nil {
		return ""
	}
	return r.ID
}

func missingHashes(ss *store.Site, files []protocol.File) []string {
	seen := map[string]bool{}
	missing := []string{}
	for _, f := range files {
		if seen[f.Hash] {
			continue
		}
		seen[f.Hash] = true
		if !ss.HasBlob(f.Hash) {
			missing = append(missing, f.Hash)
		}
	}
	sort.Strings(missing)
	return missing
}

// baseCheck implements §3.3.
func (s *Service) baseCheck(ss *store.Site, head *store.Revision, m protocol.Manifest) (protocol.BaseCheck, error) {
	bc := protocol.BaseCheck{Head: revID(head)}
	if m.BaseRevision == bc.Head {
		return bc, nil
	}
	bc.Stale = true
	if head == nil {
		return bc, nil
	}
	clients, err := ss.Clients()
	if err != nil {
		return bc, err
	}
	lastPush := clients[m.Client.ID].LastPush // zero for an unknown client
	bc.Regressions = Regressions(head.Files, m.Files, lastPush)
	return bc, nil
}

// Regressions lists the changes a manifest would make that look like rolling
// content back: replacing a file with an older version, or deleting a file
// that changed after the client's last successful push. Both slices must be
// sorted by path. The generated hints file is not user content and is
// ignored.
func Regressions(stored, incoming []protocol.File, lastPush time.Time) []protocol.Regression {
	stored, incoming = withoutHints(stored), withoutHints(incoming)
	var out []protocol.Regression
	i, j := 0, 0
	for i < len(stored) || j < len(incoming) {
		switch {
		case j == len(incoming) || (i < len(stored) && stored[i].Path < incoming[j].Path):
			st := stored[i]
			if st.MTime.After(lastPush) {
				out = append(out, protocol.Regression{Path: st.Path, Kind: protocol.RegressionRemoved, StoredMTime: st.MTime})
			}
			i++
		case i == len(stored) || incoming[j].Path < stored[i].Path:
			j++
		default:
			st, in := stored[i], incoming[j]
			if st.Hash != in.Hash && in.MTime.Before(st.MTime) {
				mt := in.MTime
				out = append(out, protocol.Regression{Path: st.Path, Kind: protocol.RegressionOlder, StoredMTime: st.MTime, IncomingMTime: &mt})
			}
			i++
			j++
		}
	}
	return out
}

func withoutHints(files []protocol.File) []protocol.File {
	for i, f := range files {
		if f.Path == protocol.HintsPath {
			return append(append([]protocol.File(nil), files[:i]...), files[i+1:]...)
		}
	}
	return files
}

// PutBlob answers PUT /syncs/{id}/blobs/{hash}.
func (s *Service) PutBlob(site, syncID, hash string, body io.Reader) error {
	_, ss, err := s.site(site)
	if err != nil {
		return err
	}
	sy, err := ss.Sync(syncID, s.Now())
	if errors.Is(err, store.ErrNotFound) {
		return errSyncNotFound()
	}
	if err != nil {
		return err
	}
	size := int64(-1)
	for _, f := range sy.Manifest.Files {
		if f.Hash == hash {
			size = f.Size
			break
		}
	}
	if size < 0 {
		return newError(http.StatusBadRequest, protocol.ErrUnexpectedBlob, "hash is not part of this sync's manifest")
	}
	if ss.HasBlob(hash) {
		return nil
	}
	switch err := ss.PutBlob(hash, size, body); {
	case errors.Is(err, store.ErrHashMismatch):
		return newError(http.StatusBadRequest, protocol.ErrHashMismatch, "uploaded content does not match its hash")
	case errors.Is(err, store.ErrSizeMismatch):
		return newError(http.StatusBadRequest, protocol.ErrHashMismatch, "uploaded content does not match the manifest size")
	default:
		return err
	}
}

// Commit answers POST /syncs/{id}/commit.
func (s *Service) Commit(site, syncID string, req protocol.CommitRequest) (protocol.CommitResponse, error) {
	sc, ss, err := s.site(site)
	if err != nil {
		return protocol.CommitResponse{}, err
	}
	unlock, err := ss.Lock()
	if err != nil {
		return protocol.CommitResponse{}, err
	}
	defer unlock()

	now := s.Now().UTC()
	sy, err := ss.Sync(syncID, now)
	if errors.Is(err, store.ErrNotFound) {
		return protocol.CommitResponse{}, errSyncNotFound()
	}
	if err != nil {
		return protocol.CommitResponse{}, err
	}
	head, err := ss.HeadRevision()
	if err != nil {
		return protocol.CommitResponse{}, err
	}
	if revID(head) != sy.Head {
		_ = ss.DeleteSync(syncID)
		return protocol.CommitResponse{}, newError(http.StatusConflict, protocol.ErrRevisionChanged,
			"another client committed %s since this sync started; start a new sync", revID(head))
	}
	rules := sc.Rules()
	if sy.Manifest.RulesHash != rules.Hash() {
		_ = ss.DeleteSync(syncID)
		return protocol.CommitResponse{}, newError(http.StatusConflict, protocol.ErrRulesChanged, "publish rules changed; fetch site info and start a new sync")
	}
	if missing := missingHashes(ss, sy.Manifest.Files); len(missing) > 0 {
		return protocol.CommitResponse{}, newError(http.StatusConflict, protocol.ErrMissingBlobs,
			"%d blobs have not been uploaded", len(missing)).withDetails(missing)
	}

	bc, err := s.baseCheck(ss, head, sy.Manifest)
	if err != nil {
		return protocol.CommitResponse{}, err
	}
	if len(bc.Regressions) > 0 && !req.Force {
		return protocol.CommitResponse{}, newError(http.StatusConflict, protocol.ErrStaleState,
			"%s; confirm to commit anyway", summarize(bc.Regressions)).withDetails(bc.Regressions)
	}

	files, warnings, err := s.gate2(ss, rules, sy.Manifest.Files)
	if err != nil {
		return protocol.CommitResponse{}, err
	}
	if bc.Stale {
		msg := "another client pushed since this device's last sync; no older content would be restored"
		if len(bc.Regressions) > 0 {
			msg = "committed after confirmation: " + summarize(bc.Regressions)
		}
		warnings = append(warnings, protocol.Warning{Code: protocol.WarnStaleBase, Message: msg})
	}

	if warnings == nil {
		warnings = []protocol.Warning{}
	}
	if head != nil && sameFiles(head.Files, files) {
		// Nothing changed: don't create a revision or a build, but remember
		// that this client is in sync (for the stale-state check).
		if err := ss.RecordClientPush(sy.Manifest.Client, head.ID, now); err != nil {
			return protocol.CommitResponse{}, err
		}
		_ = ss.DeleteSync(syncID)
		return protocol.CommitResponse{Revision: head.ID, Unchanged: true, Warnings: warnings}, nil
	}

	rev, err := ss.Commit(store.CommitParams{
		ExpectedHead: sy.Head,
		Client:       sy.Manifest.Client,
		Files:        files,
		Now:          now,
		KeepRevs:     sc.Retention.Revisions,
	})
	if errors.Is(err, store.ErrHeadMoved) {
		return protocol.CommitResponse{}, newError(http.StatusConflict, protocol.ErrRevisionChanged, "another client committed meanwhile; start a new sync")
	}
	if err != nil {
		return protocol.CommitResponse{}, err
	}
	_ = ss.DeleteSync(syncID)
	s.log.Info("commit", "site", site, "revision", rev.ID, "files", len(files), "client", sy.Manifest.Client.Name, "warnings", len(warnings))

	bs := s.queue.Enqueue(site, rev.ID)
	return protocol.CommitResponse{Revision: rev.ID, BuildID: bs.ID, Warnings: warnings}, nil
}

// sameFiles compares two path-sorted file lists by path and content. Only
// mtimes differing doesn't count as a change.
func sameFiles(a, b []protocol.File) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Hash != b[i].Hash {
			return false
		}
	}
	return true
}

func summarize(rs []protocol.Regression) string {
	older, removed := 0, 0
	for _, r := range rs {
		if r.Kind == protocol.RegressionOlder {
			older++
		} else {
			removed++
		}
	}
	var parts []string
	if older > 0 {
		parts = append(parts, plural(older, "note would revert", "notes would revert")+" to an older version")
	}
	if removed > 0 {
		parts = append(parts, plural(removed, "note changed since this device's last push would be removed", "notes changed since this device's last push would be removed"))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// gate2 re-evaluates the publish rules on every note and drops the ones that
// fail (§5.2). Attachments and reserved files pass; their reachability is
// decided at build time.
func (s *Service) gate2(ss *store.Site, rules protocol.Rules, files []protocol.File) ([]protocol.File, []protocol.Warning, error) {
	var keep []protocol.File
	var warnings []protocol.Warning
	for _, f := range files {
		if !protocol.IsNote(f.Path) {
			keep = append(keep, f)
			continue
		}
		src, err := readBlob(ss, f.Hash)
		if err != nil {
			return nil, nil, err
		}
		d, m := publish.EvaluateSource(rules, f.Path, src)
		if !d.Published {
			msg := "pushed but not public under the server's rules (" + d.Reason + "); not published"
			if m.FrontmatterErr != nil {
				msg = "frontmatter could not be read (" + m.FrontmatterErr.Error() + "); not published"
			}
			warnings = append(warnings, protocol.Warning{Code: protocol.WarnGateDisagreement, Path: f.Path, Message: msg})
			continue
		}
		keep = append(keep, f)
	}
	return keep, warnings, nil
}

func readBlob(ss *store.Site, hash string) ([]byte, error) {
	f, err := ss.OpenBlob(hash)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// BuildStatus answers GET /builds/{id}, waiting up to wait for the build to
// finish.
func (s *Service) BuildStatus(ctx context.Context, site, id string, wait time.Duration) (protocol.BuildStatus, error) {
	_, ss, err := s.site(site)
	if err != nil {
		return protocol.BuildStatus{}, err
	}
	if wait > 0 {
		ctx, cancel := context.WithTimeout(ctx, wait)
		defer cancel()
		if st, ok := s.queue.Wait(ctx, site, id); ok {
			return st, nil
		}
	} else if st, ok := s.queue.Status(site, id); ok {
		return st, nil
	}
	if st, ok := build.LastStatus(ss.Dir()); ok && st.ID == id {
		return st, nil
	}
	return protocol.BuildStatus{}, newError(http.StatusNotFound, protocol.ErrNotFound, "unknown build")
}

// Run performs background maintenance until ctx ends: garbage collection and
// noticing commits made by another process (kvist rollback).
func (s *Service) Run(ctx context.Context, gcEvery, pollEvery time.Duration) {
	gcT := time.NewTicker(gcEvery)
	pollT := time.NewTicker(pollEvery)
	defer gcT.Stop()
	defer pollT.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-gcT.C:
			s.GCAll()
		case <-pollT.C:
			for _, sc := range s.cfg.Sites {
				ss, err := s.store.Site(sc.ID)
				if err != nil {
					continue
				}
				if head, err := ss.Head(); err == nil && head != "" {
					s.queue.EnqueueIfNew(sc.ID, head)
				}
			}
		}
	}
}

// GCAll collects garbage on every site.
func (s *Service) GCAll() {
	for _, sc := range s.cfg.Sites {
		ss, err := s.store.Site(sc.ID)
		if err != nil {
			continue
		}
		unlock, err := ss.Lock()
		if err != nil {
			s.log.Error("gc lock", "site", sc.ID, "err", err)
			continue
		}
		st, err := ss.GC(s.Now())
		unlock()
		if err != nil {
			s.log.Error("gc", "site", sc.ID, "err", err)
			continue
		}
		if st.DeletedBlobs > 0 || st.ExpiredSyncs > 0 {
			s.log.Info("gc", "site", sc.ID, "deleted_blobs", st.DeletedBlobs, "expired_syncs", st.ExpiredSyncs)
		}
	}
}
