// Package cdn purges CDN caches after a build, so an unpublished page
// disappears from the edge right away instead of when its cache expires.
package cdn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CloudflareAPI is the API base; tests replace it.
var CloudflareAPI = "https://api.cloudflare.com/client/v4"

// PurgeCloudflare purges a zone's whole cache. Pages are cheap to refetch
// and attachments have immutable, content-hashed URLs, so purging
// everything is simple and never leaves a stale page behind.
func PurgeCloudflare(ctx context.Context, zoneID, token string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]bool{"purge_everything": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, CloudflareAPI+"/zones/"+zoneID+"/purge_cache", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = json.Unmarshal(b, &out)
	if resp.StatusCode != http.StatusOK || !out.Success {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return fmt.Errorf("cloudflare purge failed: %s", msg)
	}
	return nil
}
