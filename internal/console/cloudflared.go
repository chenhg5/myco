package console

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Opening a workspace on this computer to the internet should be one click:
// the Cloudflare temporary address needs no account, only cloudflared, so
// the App gets it itself when it is missing: with Homebrew on a Mac that has
// it, else Cloudflare's own release from GitHub, checked against the SHA-256
// GitHub publishes for the file.

// cloudflaredAsset is the release file for this system.
func cloudflaredAsset(goos, goarch string) (string, bool) {
	switch goos {
	case "darwin":
		if goarch == "arm64" || goarch == "amd64" {
			return "cloudflared-darwin-" + goarch + ".tgz", true
		}
	case "linux":
		if goarch == "arm64" || goarch == "amd64" || goarch == "arm" {
			return "cloudflared-linux-" + goarch, true
		}
	case "windows":
		if goarch == "amd64" || goarch == "386" {
			return "cloudflared-windows-" + goarch + ".exe", true
		}
	}
	return "", false
}

// cloudflaredAPI is Cloudflare's latest release on GitHub (tests replace it).
var cloudflaredAPI = "https://api.github.com/repos/cloudflare/cloudflared/releases/latest"

// EnsureCloudflared returns cloudflared's path, getting it first if needed.
func (l *Local) EnsureCloudflared(ctx context.Context) (string, error) {
	if p := l.cloudflaredPath(); p != "" {
		return p, nil
	}
	if runtime.GOOS == "darwin" {
		if brew := l.find("brew", "/opt/homebrew/bin/brew", "/usr/local/bin/brew"); brew != "" {
			bctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
			cmd := exec.CommandContext(bctx, brew, "install", "cloudflared")
			cmd.Env = append(l.env(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_INSTALL_CLEANUP=1")
			err := cmd.Run()
			cancel()
			if p := l.cloudflaredPath(); err == nil && p != "" {
				return p, nil
			}
			// Homebrew failed (no network to its mirrors, permissions):
			// Cloudflare's own release below.
		}
	}
	return l.downloadCloudflared(ctx, filepath.Join(l.Home, ".local", "bin"))
}

func (l *Local) downloadCloudflared(ctx context.Context, dir string) (string, error) {
	name, ok := cloudflaredAsset(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return "", fmt.Errorf("no cloudflared for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	hc := &http.Client{Timeout: 15 * time.Minute}
	var rel struct {
		Assets []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := getJSON(ctx, hc, cloudflaredAPI, &rel); err != nil {
		return "", fmt.Errorf("cannot look up cloudflared: %w", err)
	}
	var url, want string
	for _, a := range rel.Assets {
		if a.Name == name {
			url, want = a.URL, strings.TrimPrefix(a.Digest, "sha256:")
		}
	}
	if url == "" {
		return "", fmt.Errorf("cloudflared's latest release has no %s", name)
	}
	if len(want) != 64 {
		return "", errors.New("cloudflared's release does not say the file's SHA-256, so it is not installed; install it yourself (brew install cloudflared)")
	}
	data, err := getBytes(ctx, hc, url)
	if err != nil {
		return "", fmt.Errorf("downloading cloudflared: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.ToLower(want) {
		return "", errors.New("the downloaded cloudflared does not match its SHA-256; not installed")
	}
	bin := data
	if strings.HasSuffix(name, ".tgz") {
		if bin, err = fromTgz(data, "cloudflared"); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, exeName("cloudflared"))
	tmp := target + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return target, nil
}

func getJSON(ctx context.Context, hc *http.Client, url string, v any) error {
	b, err := getBytes(ctx, hc, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func getBytes(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "myco-app")
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 200<<20))
}

// fromTgz is the file named name in a .tgz.
func fromTgz(data []byte, name string) ([]byte, error) {
	zr, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s is not in the archive", name)
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) == name && h.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
}
