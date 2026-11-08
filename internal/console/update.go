package console

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/release"
)

// AppAsset is the release asset holding the app for a Mac architecture:
// a zip of myco.app (made with ditto --keepParent).
func AppAsset(arch string) string { return "myco-app-darwin-" + arch + ".zip" }

const appZipMax = 300 << 20

// Updater replaces the running app with a newer release and reopens it.
type Updater struct {
	Version string
	// Bundle is the running myco.app; "" when not running from one.
	Bundle string
	Arch   string // default runtime.GOARCH
	HTTP   *http.Client
	// API and Download are the release API and download base
	// (release.APIURL / release.DownloadURL honour the env overrides).
	API, Download string

	mu       sync.Mutex
	relaunch func()
	busy     bool
}

// SetRelaunch sets how the app quits and reopens itself (the window app
// sets it; without it the app cannot update itself, e.g. with --serve).
func (u *Updater) SetRelaunch(fn func()) {
	u.mu.Lock()
	u.relaunch = fn
	u.mu.Unlock()
}

func (u *Updater) arch() string {
	if u.Arch != "" {
		return u.Arch
	}
	return runtime.GOARCH
}

func (u *Updater) client(timeout time.Duration) *http.Client {
	if u.HTTP != nil {
		return u.HTTP
	}
	return &http.Client{Timeout: timeout}
}

// UpdateInfo is GET /app/update.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	// CanInstall says whether the app can replace itself; Reason says why not.
	CanInstall bool   `json:"can_install"`
	Reason     string `json:"reason,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (u *Updater) canInstall() (bool, string) {
	u.mu.Lock()
	relaunch := u.relaunch
	u.mu.Unlock()
	switch {
	case u.Bundle == "":
		return false, "not running from myco.app"
	case relaunch == nil:
		return false, "only the app window can update itself"
	}
	return true, ""
}

func (u *Updater) check(ctx context.Context) UpdateInfo {
	info := UpdateInfo{Current: u.Version}
	info.CanInstall, info.Reason = u.canInstall()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	latest, err := release.Latest(ctx, u.client(20*time.Second), u.API)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Latest = latest
	info.Available = release.Newer(latest, u.Version)
	if _, known := release.Parse(u.Version); !known {
		// A development build: offer the release, never insist.
		info.Available = true
	}
	return info
}

func (b *Backend) getUpdate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, b.Update.check(r.Context()))
}

// postUpdate downloads the latest release of the app, checks it against
// the release's SHA256SUMS, puts it in place of the running bundle and
// reopens the app.
func (b *Backend) postUpdate(w http.ResponseWriter, r *http.Request) {
	u := b.Update
	if ok, why := u.canInstall(); !ok {
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "unavailable", Message: why})
		return
	}
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "busy", Message: "an update is already being installed"})
		return
	}
	u.busy = true
	relaunch := u.relaunch
	u.mu.Unlock()
	defer func() { u.mu.Lock(); u.busy = false; u.mu.Unlock() }()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	hc := u.client(30 * time.Minute)
	latest, err := release.Latest(ctx, hc, u.API)
	if err != nil {
		writeErr(w, fmt.Errorf("cannot find the latest release: %w", err))
		return
	}
	zipped, err := release.FetchAsset(ctx, hc, u.Download, latest, AppAsset(u.arch()), appZipMax)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := installBundle(u.Bundle, zipped); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installed": latest, "relaunching": true})
	// Let the answer reach the page before the app quits.
	go func() {
		time.Sleep(500 * time.Millisecond)
		relaunch()
	}()
}

// installBundle unpacks a zip holding myco.app next to bundle and swaps
// it in; the old bundle is kept until the new one is in place.
func installBundle(bundle string, zipped []byte) error {
	parent := filepath.Dir(bundle)
	tmp, err := os.MkdirTemp(parent, ".myco-update-")
	if err != nil {
		return fmt.Errorf("cannot write next to %s (permissions?): %w", bundle, err)
	}
	defer os.RemoveAll(tmp)
	if err := unzipTo(tmp, zipped); err != nil {
		return err
	}
	fresh := filepath.Join(tmp, "myco.app")
	if st, err := os.Stat(filepath.Join(fresh, "Contents", "MacOS", "myco-app")); err != nil || st.Mode()&0o111 == 0 {
		return errors.New("the downloaded app is incomplete (no Contents/MacOS/myco-app)")
	}
	old := filepath.Join(tmp, "old.app")
	if err := os.Rename(bundle, old); err != nil {
		return fmt.Errorf("cannot move the current app aside: %w", err)
	}
	if err := os.Rename(fresh, bundle); err != nil {
		_ = os.Rename(old, bundle)
		return fmt.Errorf("cannot put the new app in place: %w", err)
	}
	return nil
}

// unzipTo extracts a zip into dir, refusing paths that leave it, and
// skipping macOS metadata (__MACOSX/, ._*).
func unzipTo(dir string, zipped []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		return fmt.Errorf("the downloaded app is not a zip: %w", err)
	}
	root := filepath.Clean(dir) + string(os.PathSeparator)
	for _, f := range zr.File {
		name := f.Name
		if strings.HasPrefix(name, "__MACOSX/") || strings.HasPrefix(filepath.Base(name), "._") {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if !strings.HasPrefix(target+string(os.PathSeparator), root) || filepath.IsAbs(name) {
			return fmt.Errorf("unsafe path in the downloaded app: %s", name)
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			link, err := readAll(f)
			if err != nil {
				return err
			}
			dest := filepath.Join(filepath.Dir(target), string(link))
			if filepath.IsAbs(string(link)) || !strings.HasPrefix(dest+string(os.PathSeparator), root) {
				return fmt.Errorf("unsafe link in the downloaded app: %s", name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(string(link), target); err != nil {
				return err
			}
		default:
			b, err := readAll(f)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, b, mode.Perm()|0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

func readAll(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, appZipMax))
}
