// Package build runs site builds. This file holds the per-site build queue:
// builds are serialized per site and coalesced (a queued build that has not
// started yet is superseded by a newer one).
package build

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/klppl/kvist/internal/protocol"
)

// Builder builds one revision of a site.
type Builder interface {
	Build(ctx context.Context, site, revision, buildID string) ([]protocol.Warning, error)
}

// BuilderFunc adapts a function to Builder.
type BuilderFunc func(ctx context.Context, site, revision, buildID string) ([]protocol.Warning, error)

// Build implements Builder.
func (f BuilderFunc) Build(ctx context.Context, site, revision, buildID string) ([]protocol.Warning, error) {
	return f(ctx, site, revision, buildID)
}

// historySize is how many finished builds per site stay queryable in memory.
const historySize = 50

// Queue schedules builds.
type Queue struct {
	builder   Builder
	statusDir func(site string) string // where status.json goes; nil to skip
	now       func() time.Time
	log       *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu    sync.Mutex
	sites map[string]*siteQueue
}

type entry struct {
	status protocol.BuildStatus
	done   chan struct{}
}

type siteQueue struct {
	running *entry
	pending *entry
	byID    map[string]*entry
	order   []string // finished ids, oldest first
	lastRev string   // revision of the most recently enqueued build
	wake    chan struct{}
}

// NewQueue starts a queue. statusDir may be nil.
func NewQueue(b Builder, statusDir func(site string) string, log *slog.Logger) *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	if log == nil {
		log = slog.Default()
	}
	return &Queue{
		builder:   b,
		statusDir: statusDir,
		now:       time.Now,
		log:       log,
		ctx:       ctx,
		cancel:    cancel,
		sites:     map[string]*siteQueue{},
	}
}

// Close stops the workers and waits for running builds to notice.
func (q *Queue) Close() {
	q.cancel()
	q.wg.Wait()
}

func (q *Queue) site(site string) *siteQueue {
	sq, ok := q.sites[site]
	if !ok {
		sq = &siteQueue{byID: map[string]*entry{}, wake: make(chan struct{}, 1)}
		q.sites[site] = sq
		q.wg.Add(1)
		go q.worker(site, sq)
	}
	return sq
}

// Enqueue schedules a build of revision and returns its status. If a build
// of the same revision is already queued or running, that one is returned.
func (q *Queue) Enqueue(site, revision string) protocol.BuildStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	sq := q.site(site)
	for _, e := range []*entry{sq.pending, sq.running} {
		if e != nil && e.status.Revision == revision {
			return e.status
		}
	}
	now := q.now().UTC()
	id := fmt.Sprintf("%s-%s", now.Format("20060102T150405Z"), revision)
	for n := 2; sq.byID[id] != nil; n++ {
		id = fmt.Sprintf("%s-%s-%d", now.Format("20060102T150405Z"), revision, n)
	}
	e := &entry{
		status: protocol.BuildStatus{ID: id, Revision: revision, State: protocol.BuildQueued, QueuedAt: now, Warnings: []protocol.Warning{}},
		done:   make(chan struct{}),
	}
	if old := sq.pending; old != nil {
		old.status.State = protocol.BuildSuperseded
		old.status.SupersededBy = id
		fin := now
		old.status.FinishedAt = &fin
		q.finish(sq, old)
	}
	sq.pending = e
	sq.byID[id] = e
	sq.lastRev = revision
	select {
	case sq.wake <- struct{}{}:
	default:
	}
	return e.status
}

// EnqueueIfNew enqueues a build unless revision was the last one enqueued.
// It is used when noticing commits made by another process.
func (q *Queue) EnqueueIfNew(site, revision string) {
	q.mu.Lock()
	sq := q.site(site)
	seen := sq.lastRev == revision
	q.mu.Unlock()
	if !seen {
		q.Enqueue(site, revision)
	}
}

// finish records a final state; q.mu must be held.
func (q *Queue) finish(sq *siteQueue, e *entry) {
	close(e.done)
	sq.order = append(sq.order, e.status.ID)
	for len(sq.order) > historySize {
		delete(sq.byID, sq.order[0])
		sq.order = sq.order[1:]
	}
}

func (q *Queue) worker(site string, sq *siteQueue) {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case <-sq.wake:
		}
		for {
			q.mu.Lock()
			e := sq.pending
			if e == nil {
				q.mu.Unlock()
				break
			}
			sq.pending, sq.running = nil, e
			start := q.now().UTC()
			e.status.State, e.status.StartedAt = protocol.BuildRunning, &start
			st := e.status
			q.mu.Unlock()

			warnings, err := q.builder.Build(q.ctx, site, st.Revision, st.ID)

			q.mu.Lock()
			end := q.now().UTC()
			e.status.FinishedAt = &end
			if warnings != nil {
				e.status.Warnings = warnings
			}
			if err != nil {
				e.status.State, e.status.Error = protocol.BuildFailed, err.Error()
				q.log.Error("build failed", "site", site, "build", st.ID, "err", err)
			} else {
				e.status.State = protocol.BuildSucceeded
				q.log.Info("build succeeded", "site", site, "build", st.ID, "warnings", len(e.status.Warnings))
			}
			sq.running = nil
			final := e.status
			q.finish(sq, e)
			q.mu.Unlock()
			q.writeStatus(site, final)
		}
	}
}

func (q *Queue) writeStatus(site string, st protocol.BuildStatus) {
	if q.statusDir == nil {
		return
	}
	dir := q.statusDir(site)
	b, _ := json.MarshalIndent(st, "", "  ")
	tmp := filepath.Join(dir, ".status.json.tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err == nil {
		err = os.Rename(tmp, filepath.Join(dir, "status.json"))
		if err != nil {
			q.log.Error("write build status", "site", site, "err", err)
		}
	}
}

// LastStatus reads the persisted status of the last finished build.
func LastStatus(siteDir string) (protocol.BuildStatus, bool) {
	var st protocol.BuildStatus
	b, err := os.ReadFile(filepath.Join(siteDir, "status.json"))
	if err != nil || json.Unmarshal(b, &st) != nil {
		return st, false
	}
	return st, true
}

// Status returns the status of a build known to this process.
func (q *Queue) Status(site, id string) (protocol.BuildStatus, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	sq, ok := q.sites[site]
	if !ok {
		return protocol.BuildStatus{}, false
	}
	e, ok := sq.byID[id]
	if !ok {
		return protocol.BuildStatus{}, false
	}
	return e.status, true
}

// Wait blocks until the build is done or ctx ends, then returns its status.
func (q *Queue) Wait(ctx context.Context, site, id string) (protocol.BuildStatus, bool) {
	q.mu.Lock()
	sq, ok := q.sites[site]
	var e *entry
	if ok {
		e = sq.byID[id]
	}
	q.mu.Unlock()
	if e == nil {
		return protocol.BuildStatus{}, false
	}
	select {
	case <-e.done:
	case <-ctx.Done():
	}
	return q.Status(site, id)
}
