package sc4

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestSetupDownloadRetriesInterruptedResource(t *testing.T) {
	const payload = "verified resource"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		if requests.Add(1) == 1 {
			_, _ = fmt.Fprint(w, payload[:4])
			return
		}
		_, _ = fmt.Fprint(w, payload)
	}))
	defer server.Close()

	dir := t.TempDir()
	asked := false
	deps := setupDependencies{
		ctx: context.Background(),
		dir: dir,
		emit: func(event installEvent) {
			if event.Type == "retry-resource" {
				asked = true
			}
		},
		waitForRetry: func() error {
			if !asked || requests.Load() != 1 {
				t.Fatal("Setup did not wait for Retry after the interrupted download")
			}
			if _, err := os.Stat(filepath.Join(dir, "resource.zip")); !os.IsNotExist(err) {
				t.Fatalf("Interrupted download was published: %v", err)
			}
			return nil
		},
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	output, err := deps.download(1, "Downloading test resource", "resource.zip", server.URL, digest, 1024)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != payload || !asked || requests.Load() != 2 {
		t.Fatalf("Download did not recover: content %q, error %v, requests %d", data, err, requests.Load())
	}
}
