package pushclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/klppl/kvist/internal/protocol"
)

// ClientVersion is reported to the server.
var ClientVersion = "dev"

// Options configure a push.
type Options struct {
	Dir        string // vault folder
	Force      bool   // accept a stale-state commit
	Wait       bool   // wait for the build to finish
	DeviceName string
	// StatePath stores the client id and last pushed revision. Defaults to
	// <user config dir>/kvist/push-state.json.
	StatePath string
	// Parallel uploads (default 4).
	Parallel int
	// Progress, if set, is called for each uploaded blob.
	Progress func(done, total int)
}

// Result summarizes a push.
type Result struct {
	Scan      *Scan
	Revision  string
	Unchanged bool // nothing changed since HEAD; no build was started
	BuildID   string
	Uploaded  int
	Warnings  []protocol.Warning
	Build     *protocol.BuildStatus
	Stale     protocol.BaseCheck
}

// StaleError is returned when the server reports that the push would roll
// content back (§3.3). Retry with Options.Force after confirming.
type StaleError struct {
	Message     string
	Regressions []protocol.Regression
}

func (e *StaleError) Error() string { return e.Message }

// maxAttempts bounds retries after revision_changed / rules_changed /
// sync_expired.
const maxAttempts = 3

// Push publishes a vault folder: fetch rules, scan, sync, upload, commit.
func Push(ctx context.Context, c *Client, opts Options) (*Result, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	if protocol.Version < info.Protocol.Min || protocol.Version > info.Protocol.Max {
		return nil, fmt.Errorf("server speaks protocol %d–%d, this client speaks %d; update the %s",
			info.Protocol.Min, info.Protocol.Max, protocol.Version, olderSide(info.Protocol))
	}

	stateKey, err := stateKey(c, opts.Dir)
	if err != nil {
		return nil, err
	}
	statePath := opts.StatePath
	if statePath == "" {
		cfg, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		statePath = filepath.Join(cfg, "kvist", "push-state.json")
	}
	states, err := loadStates(statePath)
	if err != nil {
		return nil, err
	}
	st := states[stateKey]
	if st.ClientID == "" {
		st.ClientID = randomID()
	}
	name := opts.DeviceName
	if name == "" {
		name, _ = os.Hostname()
	}
	client := protocol.Client{ID: st.ClientID, Name: name, Platform: runtime.GOOS, Version: ClientVersion}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		res, err := pushOnce(ctx, c, opts, client, st.BaseRevision)
		switch {
		case err == nil:
			st.BaseRevision = res.Revision
			states[stateKey] = st
			if err := saveStates(statePath, states); err != nil {
				return res, fmt.Errorf("pushed %s but could not save state: %w", res.Revision, err)
			}
			if opts.Wait && res.BuildID != "" {
				bs, err := c.WaitBuild(ctx, res.BuildID)
				if err != nil {
					return res, fmt.Errorf("waiting for build: %w", err)
				}
				res.Build = &bs
			}
			return res, nil
		case IsCode(err, protocol.ErrRevisionChanged), IsCode(err, protocol.ErrRulesChanged), IsCode(err, protocol.ErrSyncExpired):
			lastErr = err
			continue
		default:
			return nil, err
		}
	}
	return nil, fmt.Errorf("giving up after %d attempts: %w", maxAttempts, lastErr)
}

func olderSide(r protocol.VersionRange) string {
	if protocol.Version < r.Min {
		return "client"
	}
	return "server"
}

func pushOnce(ctx context.Context, c *Client, opts Options, client protocol.Client, base string) (*Result, error) {
	site, err := c.SiteInfo(ctx)
	if err != nil {
		return nil, err
	}
	scan, err := ScanDir(opts.Dir, site.Rules)
	if err != nil {
		return nil, err
	}
	if len(scan.Files) > site.Limits.MaxFiles {
		return nil, fmt.Errorf("%d files to publish; the server allows %d", len(scan.Files), site.Limits.MaxFiles)
	}
	for _, f := range scan.Files {
		if f.Size > site.Limits.MaxFileSize {
			return nil, fmt.Errorf("%s is %d bytes; the server allows %d", f.Path, f.Size, site.Limits.MaxFileSize)
		}
	}
	sy, err := c.StartSync(ctx, protocol.Manifest{
		BaseRevision: base,
		RulesHash:    site.RulesHash,
		Client:       client,
		Files:        scan.Files,
	})
	if err != nil {
		return nil, err
	}
	if err := upload(ctx, c, scan, sy, opts); err != nil {
		return nil, err
	}
	cr, err := c.Commit(ctx, sy.SyncID, opts.Force)
	if err != nil {
		var pe *protocol.Error
		if errors.As(err, &pe) && pe.Code == protocol.ErrStaleState {
			se := &StaleError{Message: pe.Message}
			_ = json.Unmarshal(pe.Details, &se.Regressions)
			return nil, se
		}
		return nil, err
	}
	return &Result{
		Scan:      scan,
		Revision:  cr.Revision,
		Unchanged: cr.Unchanged,
		BuildID:   cr.BuildID,
		Uploaded:  len(sy.Missing),
		Warnings:  cr.Warnings,
		Stale:     sy.BaseCheck,
	}, nil
}

func upload(ctx context.Context, c *Client, scan *Scan, sy protocol.SyncResponse, opts Options) error {
	if len(sy.Missing) == 0 {
		return nil
	}
	byHash := map[string]string{}
	for _, f := range scan.Files {
		byHash[f.Hash] = f.Path
	}
	par := opts.Parallel
	if par <= 0 {
		par = 4
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu       sync.Mutex
		firstErr error
		done     int
		wg       sync.WaitGroup
		work     = make(chan string)
	)
	for i := 0; i < par; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range work {
				err := func() error {
					p, ok := byHash[h]
					if !ok {
						return fmt.Errorf("server asked for unknown hash %s", h)
					}
					r, err := scan.Open(p)
					if err != nil {
						return err
					}
					defer r.Close()
					if err := c.PutBlob(ctx, sy.SyncID, h, r); err != nil {
						return fmt.Errorf("upload %s: %w", p, err)
					}
					return nil
				}()
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
					cancel()
				}
				done++
				if opts.Progress != nil && err == nil {
					opts.Progress(done, len(sy.Missing))
				}
				mu.Unlock()
			}
		}()
	}
	for _, h := range sy.Missing {
		select {
		case work <- h:
		case <-ctx.Done():
		}
	}
	close(work)
	wg.Wait()
	return firstErr
}

type state struct {
	ClientID     string `json:"client_id"`
	BaseRevision string `json:"base_revision"`
}

func stateKey(c *Client, dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return c.BaseURL + "|" + c.Site + "|" + abs, nil
}

func loadStates(p string) (map[string]state, error) {
	m := map[string]state{}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return m, nil
}

func saveStates(p string, m map[string]state) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
