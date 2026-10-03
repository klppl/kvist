// Package pushclient is the reference implementation of the client side of
// the sync protocol. `kvist push` uses it, the tests drive the server with it,
// and the Obsidian plugin mirrors its behavior.
package pushclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/klppl/kvist/internal/protocol"
)

// Client talks to one site on a kvist server.
type Client struct {
	BaseURL string // e.g. https://kvist.example.com
	Site    string
	Token   string
	HTTP    *http.Client
}

// New returns a client. It refuses plain http:// except to localhost.
func New(baseURL, site, token string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid server URL %q", baseURL)
	}
	switch u.Scheme {
	case "https":
	case "http":
		host := u.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return nil, fmt.Errorf("refusing to send a token over plain http to %s; use https", host)
		}
	default:
		return nil, fmt.Errorf("server URL must be http(s): %q", baseURL)
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Site:    site,
		Token:   token,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}, nil
}

func (c *Client) siteURL(parts ...string) string {
	u := c.BaseURL + "/api/v1/sites/" + url.PathEscape(c.Site)
	for _, p := range parts {
		u += "/" + url.PathEscape(p)
	}
	return u
}

func (c *Client) do(ctx context.Context, method, u string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set(protocol.HeaderProtocol, strconv.Itoa(protocol.Version))
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var eb protocol.ErrorBody
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if json.Unmarshal(b, &eb) == nil && eb.Error != nil {
			return eb.Error
		}
		return fmt.Errorf("%s %s: HTTP %d", method, u, resp.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) postJSON(ctx context.Context, u string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, u, bytes.NewReader(b), "application/json", out)
}

// Info fetches the server's protocol range.
func (c *Client) Info(ctx context.Context) (protocol.Info, error) {
	var info protocol.Info
	err := c.do(ctx, http.MethodGet, c.BaseURL+"/api/v1/info", nil, "", &info)
	return info, err
}

// SiteInfo fetches rules, limits and the current revision.
func (c *Client) SiteInfo(ctx context.Context) (protocol.SiteInfo, error) {
	var info protocol.SiteInfo
	err := c.do(ctx, http.MethodGet, c.siteURL(), nil, "", &info)
	return info, err
}

// StartSync sends a manifest.
func (c *Client) StartSync(ctx context.Context, m protocol.Manifest) (protocol.SyncResponse, error) {
	var resp protocol.SyncResponse
	err := c.postJSON(ctx, c.siteURL("syncs"), m, &resp)
	return resp, err
}

// PutBlob uploads one blob.
func (c *Client) PutBlob(ctx context.Context, syncID, hash string, body io.Reader) error {
	return c.do(ctx, http.MethodPut, c.siteURL("syncs", syncID, "blobs", hash), body, "application/octet-stream", nil)
}

// Commit commits a sync.
func (c *Client) Commit(ctx context.Context, syncID string, force bool) (protocol.CommitResponse, error) {
	var resp protocol.CommitResponse
	err := c.postJSON(ctx, c.siteURL("syncs", syncID, "commit"), protocol.CommitRequest{Force: force}, &resp)
	return resp, err
}

// BuildStatus fetches a build's status, long-polling up to wait.
func (c *Client) BuildStatus(ctx context.Context, id string, wait time.Duration) (protocol.BuildStatus, error) {
	var st protocol.BuildStatus
	u := c.siteURL("builds", id)
	if wait > 0 {
		u += "?wait=" + url.QueryEscape(wait.String())
	}
	err := c.do(ctx, http.MethodGet, u, nil, "", &st)
	return st, err
}

// WaitBuild follows a build (and whatever superseded it) until it is done.
func (c *Client) WaitBuild(ctx context.Context, id string) (protocol.BuildStatus, error) {
	for {
		st, err := c.BuildStatus(ctx, id, 30*time.Second)
		if err != nil {
			return st, err
		}
		if st.State == protocol.BuildSuperseded && st.SupersededBy != "" {
			id = st.SupersededBy
			continue
		}
		if st.Done() {
			return st, nil
		}
		if err := ctx.Err(); err != nil {
			return st, err
		}
	}
}

// IsCode reports whether err is a protocol error with the given code.
func IsCode(err error, code string) bool {
	var pe *protocol.Error
	return errors.As(err, &pe) && pe.Code == code
}
