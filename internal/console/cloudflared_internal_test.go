package console

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cloudflared comes from Cloudflare's release, checked against the SHA-256
// GitHub gives for the file; a file that does not match is not installed.
func TestDownloadCloudflared(t *testing.T) {
	name, ok := cloudflaredAsset(runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("no cloudflared for this system")
	}
	bin := []byte("#!/bin/sh\necho cloudflared\n")
	file := bin
	if strings.HasSuffix(name, ".tgz") {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(zw)
		_ = tw.WriteHeader(&tar.Header{Name: "cloudflared", Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
		_, _ = tw.Write(bin)
		_ = tw.Close()
		_ = zw.Close()
		file = buf.Bytes()
	}
	sum := sha256.Sum256(file)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": []map[string]string{
				{"name": "other", "browser_download_url": srv.URL + "/x", "digest": "sha256:00"},
				{"name": name, "browser_download_url": srv.URL + "/file", "digest": digest},
			}})
		case "/file":
			_, _ = w.Write(file)
		}
	}))
	defer srv.Close()
	old := cloudflaredAPI
	cloudflaredAPI = srv.URL + "/latest"
	t.Cleanup(func() { cloudflaredAPI = old })

	dir := t.TempDir()
	l := &Local{Home: dir}
	p, err := l.downloadCloudflared(context.Background(), filepath.Join(dir, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, bin) {
		t.Fatalf("installed %q", got)
	}
	file = append(file, 'x') // no longer matches its digest
	if _, err := l.downloadCloudflared(context.Background(), filepath.Join(dir, "bin2")); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("a file that does not match must not be installed: %v", err)
	}
}
