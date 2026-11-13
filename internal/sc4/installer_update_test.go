package sc4

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetupUpdateReportsOnlyNewerStableReleases(t *testing.T) {
	tests := []struct {
		current, release, want string
	}{
		{"v0.9.0", "v0.10.0", "v0.10.0\n"},
		{"v0.10.0", "v0.9.0", ""},
		{"v0.10.0+local", "v0.10.0", ""},
		{"v0.10.0-rc.1", "v0.10.0", "v0.10.0\n"},
		{"v0.9.0", "v0.10.0-rc.1", ""},
		{"dev", "v0.10.0", ""},
	}
	for _, tc := range tests {
		t.Run(tc.current+"/"+tc.release, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `{"tag_name":%q}`, tc.release)
			}))
			defer server.Close()
			var out bytes.Buffer
			if err := reportSetupUpdate(tc.current, server.URL, server.Client(), &out); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Fatalf("reported %q, want %q", out.String(), tc.want)
			}
		})
	}
}
