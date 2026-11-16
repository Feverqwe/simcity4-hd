package game

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
)

func TestVerifyPackagesChecksUniqueCompressedResources(t *testing.T) {
	compressed := []byte{13, 0, 0, 0, 0x10, 0xfb, 0, 0, 3, 0xff, 'a', 'b', 'c'}
	rawTexture := make([]byte, 0xfb10)
	copy(rawTexture, "SHPI")
	dbpf.Little.PutUint32(rawTexture[4:], uint32(len(rawTexture)))
	dbpf.Little.PutUint32(rawTexture[8:], 1)
	copy(rawTexture[12:], "G264demo")
	dbpf.Little.PutUint32(rawTexture[20:], 24)
	rawTexture[24] = 0x60
	dbpf.Little.PutUint16(rawTexture[28:], 4)
	dbpf.Little.PutUint16(rawTexture[30:], 4)

	for _, tc := range []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{name: "valid QFS", data: compressed},
		{name: "truncated QFS header", data: compressed[:6], wantErr: "invalid QFS header"},
		{name: "truncated QFS payload", data: compressed[:12], wantErr: "QFS command exceeds input"},
		{name: "raw texture with marker in size", data: rawTexture},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "resource.dat")
			entry := dbpf.Entry{ID: dbpf.ID{Kind: 0x7ab50e44, Group: 1, Instance: 2}, Data: tc.data}
			if err := PublishFile(path, func(w io.Writer) error {
				return dbpf.Write(w, []dbpf.Entry{entry})
			}); err != nil {
				t.Fatal(err)
			}
			err := VerifyPackages([]string{path}, io.Discard)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), entry.ID.String()) {
				t.Fatalf("expected %q for resource %s, got %v", tc.wantErr, entry.ID, err)
			}
		})
	}
}
