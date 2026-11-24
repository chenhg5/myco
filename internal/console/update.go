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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/osutil"
	"github.com/myco-ai/myco/internal/release"
)

// AppAsset is the release asset holding the app for a Mac architecture:
// a zip of myco.app (made with ditto --keepParent).
func AppAsset(arch string) string { return "myco-app-darwin-" + arch + ".zip" }

// WindowsAppAsset is the release asset holding the Windows app exe.
func WindowsAppAsset(arch string) string { return "myco-app-windows-" + arch + ".exe" }

// LinuxAppAsset is the release asset holding the Linux app (one binary
// that opens the App in the browser).
func LinuxAppAsset(arch string) string { return "myco-app-linux-" + arch }

// exeAsset is the release asset that replaces Exe on goos.
func exeAsset(goos, arch string) string {
	if goos == "windows" {
		return WindowsAppAsset(arch)
	}
	return LinuxAppAsset(arch)
}

const appZipMax = 300 << 20

// Updater replaces the running app with a newer release and reopens it.
type Updater struct {
	Version string
	// Bundle is the running myco.app; "" when not running from one.
	Bundle string
	// Exe is the running app's program on Windows and Linux ("" on a Mac):
	// the update replaces it in place (a running program can be renamed).
	Exe  string
	Arch string // default runtime.GOARCH
	OS   string // default runtime.GOOS: which program Exe is
	HTTP *http.Client
	// API and Downloads are the release API and download bases
	// (release.APIURL / release.DownloadURLs honour the env overrides):
	// GitHub and Gitee, whichever answers faster.
	API       string
	Downloads []string

	// launch starts the new program after an update on Windows (tests
	// replace it); the default starts it detached from this one.
	launch func(exe string) (waiter, error)

	mu       sync.Mutex
	relaunch func()
	busy     bool
}

// waiter is a started program: Wait returns when it exits, Kill ends it.
type waiter interface {
	Wait() error
	Kill() error
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

func (u *Updater) goos() string {
	if u.OS != "" {
		return u.OS
	}
	return runtime.GOOS
}

func (u *Updater) client(timeout time.Duration) *http.Client {
	if u.HTTP != nil {
		return u.HTTP
	}
	return &http.Client{Timeout: timeout}
}

// clients are the ways to reach GitHub, in order: as set (Settings › 代理,
// the environment's or the system's proxy), then direct when that goes
// through a proxy, so a proxy that is set but broken does not block updates.
func (u *Updater) clients(timeout time.Duration) []*http.Client {
	first := u.client(timeout)
	if u.HTTP != nil {
		return []*http.Client{first}
	}
	probe, _ := http.NewRequest(http.MethodGet, release.DefaultDownload, nil)
	if p, err := client.Proxy(probe); err != nil || p == nil {
		return []*http.Client{first}
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return []*http.Client{first}
	}
	direct := base.Clone()
	direct.Proxy = nil
	return []*http.Client{first, {Timeout: timeout, Transport: direct}}
}

// latest asks for the latest release, each way in turn.
func (u *Updater) latest(ctx context.Context, timeout time.Duration) (string, error) {
	var err error
	for _, hc := range u.clients(timeout) {
		var tag string
		if tag, err = release.Latest(ctx, hc, u.API); err == nil || ctx.Err() != nil {
			return tag, err
		}
	}
	return "", err
}

// fetch downloads and verifies a release asset, each way in turn.
func (u *Updater) fetch(ctx context.Context, tag, asset string) ([]byte, error) {
	var err error
	for _, hc := range u.clients(30 * time.Minute) {
		var b []byte
		if b, err = release.FetchAssetFrom(ctx, hc, u.Downloads, tag, asset, appZipMax); err == nil || ctx.Err() != nil || strings.Contains(err.Error(), "has no") {
			return b, err
		}
	}
	return nil, err
}

// downloadErr says what a failed download means for the person.
func downloadErr(err error) error {
	if strings.Contains(err.Error(), "has no") || strings.Contains(err.Error(), "checksum mismatch") {
		return err
	}
	return &client.Error{Status: http.StatusBadGateway, Code: "download_failed",
		Message:    "cannot download the update from GitHub or Gitee: " + err.Error(),
		Suggestion: "both may be slow or blocked on this network: turn on your proxy app, or set a proxy under Settings › Proxy, then try again",
		Retryable:  true}
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
	case u.Bundle == "" && u.Exe == "":
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
	latest, err := u.latest(ctx, 20*time.Second)
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
	latest, err := u.latest(ctx, 30*time.Second)
	if err != nil {
		writeErr(w, downloadErr(fmt.Errorf("cannot find the latest release: %w", err)))
		return
	}
	if u.Exe != "" {
		exe, err := u.fetch(ctx, latest, exeAsset(u.goos(), u.arch()))
		if err != nil {
			writeErr(w, downloadErr(err))
			return
		}
		old, err := installExe(u.Exe, exe, u.goos())
		if err != nil {
			writeErr(w, &client.Error{Status: http.StatusConflict, Code: "install_failed", Message: err.Error(),
				Suggestion: "another program (often antivirus or a sync app such as OneDrive) is holding the myco app's file; try again in a moment, or download the new version from GitHub"})
			return
		}
		if u.goos() == "windows" {
			// Start the new version here and wait for it to report in;
			// if it does not (an antivirus blocked it, it crashed), the
			// current one is put back and keeps running.
			if err := u.handOver(u.Exe); err != nil {
				if rerr := restoreExe(u.Exe, old); rerr != nil {
					err = fmt.Errorf("%w; putting the current version back failed too: %v", err, rerr)
				}
				writeErr(w, &client.Error{Status: http.StatusBadGateway, Code: "start_failed", Message: err.Error(),
					Suggestion: "the current version was kept; if an antivirus blocked the new myco app, allow it there, or download the new version from GitHub"})
				return
			}
		}
		// The myco CLI on this computer goes along; it runs on after the
		// app quits (a runner keeps running until it is restarted).
		if b.Local != nil {
			b.Local.upgradeCLIDetached()
		}
		writeJSON(w, http.StatusOK, map[string]any{"installed": latest, "relaunching": true})
		// Let the answer reach the page before the app quits (on Windows
		// the new one is already running; elsewhere relaunch starts it).
		go func() {
			time.Sleep(500 * time.Millisecond)
			relaunch()
		}()
		return
	}
	// The signed universal build when the release has it, else this Mac's own.
	zipped, err := u.fetch(ctx, latest, AppAsset("universal"))
	if err != nil && strings.Contains(err.Error(), "has no") {
		zipped, err = u.fetch(ctx, latest, AppAsset(u.arch()))
	}
	if err != nil {
		writeErr(w, downloadErr(err))
		return
	}
	if err := installBundle(u.Bundle, zipped); err != nil {
		writeErr(w, err)
		return
	}
	// The myco CLI on this computer goes along, as on Windows (its
	// services pick it up when they next restart).
	if b.Local != nil {
		b.Local.upgradeCLIDetached()
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
	if st, err := os.Stat(filepath.Join(fresh, "Contents", "MacOS", "myco-app")); err != nil || (osutil.UnixPermissions && st.Mode()&0o111 == 0) {
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

// renameTries and renameWait bound retries of a rename that failed: on
// Windows an antivirus scanning a new program, or a sync app (OneDrive)
// uploading it, holds the file for a moment.
var (
	renameTries = 20
	renameWait  = 500 * time.Millisecond
)

func renameRetry(from, to string) error {
	var err error
	for i := 0; i < renameTries; i++ {
		if err = os.Rename(from, to); err == nil || errors.Is(err, os.ErrNotExist) {
			return err
		}
		time.Sleep(renameWait)
	}
	return err
}

// installExe puts a new exe in place of the running one: the new one is
// written next to it, the running one renamed to <exe>.old (Windows allows
// that, not overwriting it), then the new one takes its name. It returns
// where the running one went (restoreExe puts it back). The old one is
// removed when the new app starts (CleanOldExe).
func installExe(exe string, data []byte, goos string) (string, error) {
	if goos == "windows" && !bytes.HasPrefix(data, []byte("MZ")) {
		return "", errors.New("the downloaded app is not a Windows program")
	}
	if goos != "windows" && !bytes.HasPrefix(data, []byte("\x7fELF")) {
		return "", errors.New("the downloaded app is not a Linux program")
	}
	fresh := exe + ".new"
	_ = os.Remove(fresh)
	if err := os.WriteFile(fresh, data, 0o755); err != nil {
		_ = os.Remove(fresh)
		return "", fmt.Errorf("cannot write next to %s (permissions?): %w", exe, err)
	}
	if st, err := os.Stat(fresh); err != nil || st.Size() != int64(len(data)) {
		// An antivirus took it away as soon as it was written.
		_ = os.Remove(fresh)
		return "", fmt.Errorf("the new app was removed right after it was written next to %s (an antivirus?)", exe)
	}
	old := exe + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		// A previous old exe still in use: set this one aside under another name.
		old = fmt.Sprintf("%s.old-%d", exe, time.Now().UnixNano())
	}
	if err := renameRetry(exe, old); err != nil {
		_ = os.Remove(fresh)
		return "", fmt.Errorf("cannot move the current app aside: %w", err)
	}
	if err := renameRetry(fresh, exe); err != nil {
		_ = renameRetry(old, exe)
		_ = os.Remove(fresh)
		return "", fmt.Errorf("cannot put the new app in place: %w", err)
	}
	return old, nil
}

// restoreExe undoes installExe: the new exe is set aside (removed at the
// next start) and the old one takes its name back.
func restoreExe(exe, old string) error {
	failed := fmt.Sprintf("%s.failed-%d", exe, time.Now().UnixNano())
	if err := renameRetry(exe, failed); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot move the new app aside: %w", err)
	}
	if err := renameRetry(old, exe); err != nil {
		return fmt.Errorf("cannot put the current app back: %w", err)
	}
	_ = os.Remove(failed)
	return nil
}

// handOverWait is how long a new version may take to report in: the first
// start of a new program can wait on an antivirus scan.
var handOverWait = 60 * time.Second

// startedMarker is the file a new version removes when it starts
// (ConfirmStarted), telling the one that started it that it came up.
func startedMarker(exe string) string { return exe + ".starting" }

// ConfirmStarted tells an update waiting for this program that it started
// (the app calls it once it is up).
func ConfirmStarted(exe string) {
	if exe != "" {
		_ = os.Remove(startedMarker(exe))
	}
}

// handOver starts the program at exe and waits until it reports in. If it
// quits first, or does not report in time, it is stopped and an error says
// why.
func (u *Updater) handOver(exe string) error {
	marker := startedMarker(exe)
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		return fmt.Errorf("cannot start the new version: %w", err)
	}
	launch := u.launch
	if launch == nil {
		launch = launchApp
	}
	p, err := launch(exe)
	if err != nil {
		_ = os.Remove(marker)
		return fmt.Errorf("cannot start the new version: %w", err)
	}
	started := func() bool {
		_, err := os.Stat(marker)
		return errors.Is(err, os.ErrNotExist)
	}
	exited := make(chan error, 1)
	go func() { exited <- p.Wait() }()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	deadline := time.NewTimer(handOverWait)
	defer deadline.Stop()
	for {
		select {
		case <-tick.C:
			if started() {
				return nil
			}
		case werr := <-exited:
			if started() {
				return nil
			}
			_ = os.Remove(marker)
			if werr != nil {
				return fmt.Errorf("the new version quit as soon as it started (%v)", werr)
			}
			return errors.New("the new version quit as soon as it started")
		case <-deadline.C:
			if started() {
				return nil
			}
			_ = p.Kill()
			select { // let it go before its file is moved
			case <-exited:
			case <-time.After(5 * time.Second):
			}
			_ = os.Remove(marker)
			return fmt.Errorf("the new version did not start within %s", handOverWait)
		}
	}
}

// CleanOldExe removes what installExe and restoreExe set aside (best
// effort: an old exe still running stays until next time).
func CleanOldExe(exe string) {
	if exe == "" {
		return
	}
	for _, pat := range []string{exe + ".old*", exe + ".failed-*"} {
		olds, _ := filepath.Glob(pat)
		for _, o := range olds {
			_ = os.Remove(o)
		}
	}
	_ = os.Remove(exe + ".new")
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

// cmdWaiter is a started command as a waiter.
type cmdWaiter struct{ cmd *exec.Cmd }

func (c cmdWaiter) Wait() error { return c.cmd.Wait() }
func (c cmdWaiter) Kill() error { return c.cmd.Process.Kill() }
