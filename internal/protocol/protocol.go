// Package protocol defines the kvist sync protocol (v1) wire types and
// constants. It is shared by the server and the reference push client; the
// normative description lives in docs/protocol.md.
package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// Version is the protocol version spoken by this build.
const Version = 1

// Supported protocol range of this build.
const (
	MinVersion = 1
	MaxVersion = 1
)

// HTTP headers.
const (
	HeaderProtocol      = "Kvist-Protocol"
	HeaderServerVersion = "Kvist-Server-Version"
)

// Reserved vault paths that may be pushed although they live under a dot
// folder. Nothing else under .kvist/ is accepted.
const (
	HintsPath      = ".kvist/links.json"
	SiteConfigPath = ".kvist/site.toml"
)

// HashPrefix prefixes every content hash on the wire.
const HashPrefix = "sha256:"

// Info is returned by GET /api/v1/info (unauthenticated).
type Info struct {
	Protocol VersionRange `json:"protocol"`
	Server   string       `json:"server"`
}

// VersionRange is an inclusive range of protocol versions.
type VersionRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Rules are the publish rules of a site. The server is the single source of
// truth; clients fetch them during the handshake and apply them as gate 1.
type Rules struct {
	AlwaysPublicFolders  []string `json:"always_public_folders"`
	ExcludeFolders       []string `json:"exclude_folders"`
	PublicTag            string   `json:"public_tag"`
	PrivateTag           string   `json:"private_tag"`
	FrontmatterKey       string   `json:"frontmatter_key"`
	AttachmentExtensions []string `json:"attachment_extensions"`
}

// Hash returns a stable hash of the rules. Clients echo it in the manifest so
// the server can detect that they evaluated stale rules.
func (r Rules) Hash() string {
	c := r
	c.AlwaysPublicFolders = sortedCopy(r.AlwaysPublicFolders)
	c.ExcludeFolders = sortedCopy(r.ExcludeFolders)
	c.AttachmentExtensions = sortedCopy(r.AttachmentExtensions)
	b, _ := json.Marshal(c) // struct field order is fixed, so this is canonical
	return HashBytes(b)
}

func sortedCopy(s []string) []string {
	out := append([]string{}, s...)
	sort.Strings(out)
	return out
}

// Limits are the server-enforced size limits for a site.
type Limits struct {
	MaxFileSize  int64 `json:"max_file_size"`
	MaxFiles     int   `json:"max_files"`
	MaxPathBytes int   `json:"max_path_bytes"`
}

// SiteInfo is returned by GET /api/v1/sites/{site}.
type SiteInfo struct {
	Site      string `json:"site"`
	BaseURL   string `json:"base_url"` // public URL of the site, for "open published page"
	Rules     Rules  `json:"rules"`
	RulesHash string `json:"rules_hash"`
	Limits    Limits `json:"limits"`
	Revision  string `json:"revision"` // current HEAD, "" if nothing committed yet
}

// Client identifies the pushing device.
type Client struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Platform string `json:"platform,omitempty"`
	Version  string `json:"version,omitempty"`
}

// File is one entry of a manifest or revision.
type File struct {
	Path  string    `json:"path"`
	Hash  string    `json:"hash"`
	Size  int64     `json:"size"`
	MTime time.Time `json:"mtime"`
}

// Manifest is the full desired state sent by POST /syncs.
type Manifest struct {
	BaseRevision string `json:"base_revision"`
	RulesHash    string `json:"rules_hash"`
	Client       Client `json:"client"`
	Files        []File `json:"files"`
}

// SyncResponse answers POST /syncs.
type SyncResponse struct {
	SyncID    string    `json:"sync_id"`
	Missing   []string  `json:"missing"`
	ExpiresAt time.Time `json:"expires_at"`
	BaseCheck BaseCheck `json:"base_check"`
}

// BaseCheck is a preview of the stale-state check (§3.3) computed when the
// sync starts. The authoritative check runs again at commit.
type BaseCheck struct {
	Head        string       `json:"head"`
	Stale       bool         `json:"stale"`
	Regressions []Regression `json:"regressions,omitempty"`
}

// Regression kinds.
const (
	RegressionOlder   = "older"   // incoming file is older than the stored one
	RegressionRemoved = "removed" // a file newer than the client's last push would be deleted
)

// Regression describes one change that would roll content back.
type Regression struct {
	Path          string     `json:"path"`
	Kind          string     `json:"kind"`
	StoredMTime   time.Time  `json:"stored_mtime"`
	IncomingMTime *time.Time `json:"incoming_mtime,omitempty"`
}

// CommitRequest is the body of POST /syncs/{id}/commit.
type CommitRequest struct {
	// Force accepts a stale-state commit after the user confirmed it.
	Force bool `json:"force,omitempty"`
}

// CommitResponse answers a successful commit.
type CommitResponse struct {
	Revision string `json:"revision"`
	// Unchanged is true when the manifest matched HEAD: no revision was
	// created and BuildID is empty.
	Unchanged bool      `json:"unchanged,omitempty"`
	BuildID   string    `json:"build_id,omitempty"`
	Warnings  []Warning `json:"warnings"`
}

// Warning codes.
const (
	WarnGateDisagreement   = "gate_disagreement"
	WarnStaleBase          = "stale_base"
	WarnInvalidFrontmatter = "invalid_frontmatter"
	WarnBuild              = "build"
)

// Warning is a non-fatal problem. Warnings may name notes and are only ever
// returned to an authenticated client of the site.
type Warning struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Build states.
const (
	BuildQueued     = "queued"
	BuildRunning    = "running"
	BuildSucceeded  = "succeeded"
	BuildFailed     = "failed"
	BuildSuperseded = "superseded"
)

// BuildStatus answers GET /builds/{id}.
type BuildStatus struct {
	ID           string     `json:"id"`
	Revision     string     `json:"revision"`
	State        string     `json:"state"`
	SupersededBy string     `json:"superseded_by,omitempty"`
	Error        string     `json:"error,omitempty"`
	Warnings     []Warning  `json:"warnings"`
	QueuedAt     time.Time  `json:"queued_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// Done reports whether the build reached a final state.
func (b BuildStatus) Done() bool {
	switch b.State {
	case BuildSucceeded, BuildFailed, BuildSuperseded:
		return true
	}
	return false
}

// Error codes.
const (
	ErrBadRequest          = "bad_request"
	ErrUnauthorized        = "unauthorized"
	ErrNotFound            = "not_found"
	ErrProtocolUnsupported = "protocol_unsupported"
	ErrInvalidManifest     = "invalid_manifest"
	ErrRulesChanged        = "rules_changed"
	ErrRevisionChanged     = "revision_changed"
	ErrStaleState          = "stale_state"
	ErrMissingBlobs        = "missing_blobs"
	ErrHashMismatch        = "hash_mismatch"
	ErrTooLarge            = "too_large"
	ErrSyncExpired         = "sync_expired"
	ErrUnexpectedBlob      = "unexpected_blob"
	ErrInternal            = "internal"
)

// Error is the error body: {"error": {...}}.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details carries structured data for some codes: []Regression for
	// stale_state, []string (hashes) for missing_blobs, []PathError for
	// invalid_manifest.
	Details json.RawMessage `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// ErrorBody wraps Error on the wire.
type ErrorBody struct {
	Error *Error `json:"error"`
}

// PathError reports an invalid manifest entry.
type PathError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// HashBytes returns the wire hash of b.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return HashPrefix + hex.EncodeToString(sum[:])
}

// ValidHash reports whether h is "sha256:" followed by 64 lowercase hex digits.
func ValidHash(h string) bool {
	hexPart, ok := strings.CutPrefix(h, HashPrefix)
	if !ok || len(hexPart) != 64 {
		return false
	}
	for i := 0; i < len(hexPart); i++ {
		c := hexPart[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
