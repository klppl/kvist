package cdn

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPurgeCloudflare(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if strings.Contains(r.URL.Path, "bad") {
			w.WriteHeader(403)
			io.WriteString(w, `{"success":false,"errors":[{"message":"Authentication error"}]}`)
			return
		}
		io.WriteString(w, `{"success":true}`)
	}))
	defer srv.Close()
	CloudflareAPI = srv.URL
	if err := PurgeCloudflare(context.Background(), "zone1", "tok"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/zones/zone1/purge_cache" || gotAuth != "Bearer tok" || gotBody != `{"purge_everything":true}` {
		t.Errorf("request: %s %s %s", gotPath, gotAuth, gotBody)
	}
	if err := PurgeCloudflare(context.Background(), "bad", "tok"); err == nil || !strings.Contains(err.Error(), "Authentication error") {
		t.Errorf("error: %v", err)
	}
}
