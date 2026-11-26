package browserautomation

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const ComponentVersion = "0.5.0"
const ComponentRelease = "https://github.com/myco-ai/myco/releases/download/browser-v" + ComponentVersion

// Component owns only the optional process and its private directory. A new
// engine can implement the same HTTP protocol without changing the App or Hub.
type Component struct {
	Home            string
	HTTP            *http.Client
	mu              sync.Mutex
	installing      bool
	process         *exec.Cmd
	input           io.WriteCloser
	done            chan struct{}
	cleanup         func()
	address, secret string
}

func (c *Component) dir() string {
	return filepath.Join(c.Home, ".myco", "components", "browser", ComponentVersion)
}

// Only explicitly compatible protocol-1 releases may be reused. Never scan
// arbitrary version directories, copy old binaries into a new version, or
// download an unreviewed release just because the App has been upgraded.
func compatibleComponentVersions() []string {
	return []string{ComponentVersion, "0.4.0", "0.3.1", "0.3.0", "0.2.0", "0.1.1", "0.1.0"}
}

// CompatibleComponentVersion is the explicit protocol-1 allowlist shared by
// the local process handshake and agent-host discovery.
func CompatibleComponentVersion(version string) bool {
	for _, allowed := range compatibleComponentVersions() {
		if version == allowed {
			return true
		}
	}
	return false
}

func (c *Component) installedComponent() (string, string, bool) {
	for _, version := range compatibleComponentVersions() {
		dir := filepath.Join(c.Home, ".myco", "components", "browser", version)
		engine, a := os.Stat(filepath.Join(dir, program("myco-browser")))
		if a == nil && engine.Mode().IsRegular() {
			return dir, version, true
		}
	}
	return c.dir(), ComponentVersion, false
}

func program(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
func browserSupported() bool {
	return (runtime.GOOS == "darwin" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64")) || (runtime.GOOS == "windows" && runtime.GOARCH == "amd64")
}
func (c *Component) statusLocked() ComponentStatus {
	if !filepath.IsAbs(c.Home) {
		return ComponentStatus{Protocol: Protocol, Version: ComponentVersion, Reason: "home_unavailable"}
	}
	if c.process != nil {
		select {
		case <-c.done:
			if c.cleanup != nil {
				c.cleanup()
			}
			_ = c.input.Close()
			c.process = nil
			c.address = ""
			c.secret = ""
		default:
		}
	}
	_, version, installed := c.installedComponent()
	s := ComponentStatus{Protocol: Protocol, Version: version, Installed: installed, Supported: browserSupported(), Running: c.process != nil}
	s.UpdateAvailable = installed && version != ComponentVersion

	if !s.Supported {
		s.Reason = "unsupported_platform"
	}
	return s
}
func (c *Component) Status() ComponentStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusLocked()
}
func (c *Component) Install(ctx context.Context) error {
	if !browserSupported() {
		return errors.New("unsupported_platform")
	}
	return c.install(ctx)
}

func (c *Component) install(ctx context.Context) error {
	if !filepath.IsAbs(c.Home) {
		return errors.New("home directory is unavailable")
	}
	c.mu.Lock()
	if c.installing {
		c.mu.Unlock()
		return ErrBusy
	}
	c.installing = true
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.installing = false; c.mu.Unlock() }()
	name := "myco-browser_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	fetch := func(name string, limit int64) ([]byte, error) {
		req, e := http.NewRequestWithContext(ctx, "GET", ComponentRelease+"/"+name, nil)
		if e != nil {
			return nil, e
		}
		hc := c.HTTP
		if hc == nil {
			hc = &http.Client{Timeout: 8 * time.Minute}
		}
		res, e := hc.Do(req)
		if e != nil {
			return nil, e
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("component download unavailable (HTTP %d); use a desktop build with published browser components", res.StatusCode)
		}
		if res.Request != nil && res.Request.URL.Scheme != "https" {
			return nil, errors.New("insecure component download")
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, limit+1))
		if e != nil {
			return nil, e
		}
		if int64(len(b)) > limit {
			return nil, errors.New("component download too large")
		}
		return b, nil
	}
	sums, e := fetch("SHA256SUMS", 64<<10)
	if e != nil {
		return e
	}
	digest := ""
	for _, line := range strings.Split(string(sums), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && strings.TrimPrefix(parts[1], "*") == name {
			digest = parts[0]
		}
	}
	if raw, e := hex.DecodeString(digest); e != nil || len(raw) != 32 {
		return errors.New("component checksum is missing")
	}
	data, e := fetch(name, 160<<20)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.ToLower(digest) {
		return errors.New("component checksum mismatch")
	}
	if e = os.MkdirAll(filepath.Dir(c.dir()), 0700); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp(filepath.Dir(c.dir()), ".install-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	if e = extractComponent(data, tmp); e != nil {
		return e
	}
	// An existing installation is kept until the new archive passed validation.
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, e = os.Stat(c.dir()); e == nil {
		return nil
	}
	if e = os.Rename(tmp, c.dir()); e != nil {
		return e
	}
	// Download, checksum and extraction failures leave the old process alive.
	// Switch only after the new immutable version directory is in place. The
	// next page call starts the new release; profiles stay in the shared store.
	c.stopLocked()
	return nil
}
func extractComponent(data []byte, dir string) error {
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return e
	}
	required := map[string]bool{program("myco-browser"): false}
	var total uint64
	for _, f := range z.File {
		name := f.Name
		if filepath.Base(name) != name || strings.ContainsAny(name, "/\\:") || f.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe component archive")
		}
		_, binary := required[name]
		if !binary && !strings.HasPrefix(name, "LICENSE") && !strings.HasPrefix(name, "NOTICE") {
			return errors.New("unexpected component file")
		}
		total += f.UncompressedSize64
		if total > 256<<20 {
			return errors.New("component archive too large")
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		dst, e := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if e != nil {
			r.Close()
			return e
		}
		n, e := io.Copy(dst, io.LimitReader(r, 128<<20+1))
		ce := dst.Close()
		r.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		if n > 128<<20 {
			return errors.New("component file too large")
		}
		if binary {
			required[name] = true
		}
	}
	for _, found := range required {
		if !found {
			return errors.New("component archive is incomplete")
		}
	}
	return nil
}
func (c *Component) Start(ctx context.Context) (ComponentStatus, error) {
	if !filepath.IsAbs(c.Home) {
		return ComponentStatus{}, errors.New("home directory is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.installing || c.process != nil {
		return c.statusLocked(), ErrBusy
	}
	if !c.statusLocked().Installed {
		return c.statusLocked(), errors.New("component_not_installed")
	}
	secret := nonce()
	cfg := map[string]any{"secret": secret, "data_dir": filepath.Join(c.Home, ".myco", "browser")}
	// The component is a child with no Hub credentials and no auto-start service.
	dir, selectedVersion, _ := c.installedComponent()
	binary := filepath.Join(dir, program("myco-browser"))
	dataDir := filepath.Join(c.Home, ".myco", "browser")
	// A failed recovery blocks only its profile; the engine checks each lease
	// and record again before opening it. Other profiles remain available.
	_ = cleanupRecordedBrowserGroups(binary, dataDir, 0)
	protectedProfiles, groupErr := hasRecordedBrowserGroups(dataDir)
	if groupErr != nil {
		return c.statusLocked(), groupErr
	}
	cmd := exec.Command(binary)
	hideComponent(cmd)
	for _, key := range browserEnvironmentKeys() {
		if v := os.Getenv(key); v != "" {
			cmd.Env = append(cmd.Env, key+"="+v)
		}
	}
	stdin, e := cmd.StdinPipe()
	if e != nil {
		return c.statusLocked(), e
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		stdin.Close()
		return c.statusLocked(), e
	}
	if e = cmd.Start(); e != nil {
		stdin.Close()
		return c.statusLocked(), e
	}
	processCleanup, e := guardComponent(cmd)
	if e != nil {
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return c.statusLocked(), e
	}
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() { processCleanup(); _ = cleanupRecordedBrowserGroups(binary, dataDir, cmd.Process.Pid) })
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); cleanup(); close(done) }()
	failed := true
	defer func() {
		if failed {
			stdin.Close()
			cleanup()
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	if e = json.NewEncoder(stdin).Encode(cfg); e != nil {
		return c.statusLocked(), e
	}
	type hello struct {
		Address       string `json:"address"`
		Protocol      int    `json:"protocol"`
		ProcessGroups int    `json:"process_groups"`
	}
	result := make(chan hello, 1)
	go func() {
		var h hello
		_ = json.NewDecoder(bufio.NewReader(io.LimitReader(stdout, 4096))).Decode(&h)
		result <- h
	}()
	var h hello
	select {
	case h = <-result:
	case <-ctx.Done():
		return c.statusLocked(), ctx.Err()
	case <-time.After(10 * time.Second):
		return c.statusLocked(), errors.New("component_start_timeout")
	}
	if protectedProfiles && h.ProcessGroups != 1 {
		return c.statusLocked(), errors.New("incompatible_component")
	}
	host, _, e := net.SplitHostPort(h.Address)
	if e != nil || host != "127.0.0.1" || h.Protocol != Protocol {
		return c.statusLocked(), errors.New("incompatible_component")
	}
	check, err := http.NewRequestWithContext(ctx, "GET", "http://"+h.Address+"/status", nil)
	if err != nil {
		return c.statusLocked(), err
	}
	check.Header.Set("Authorization", "Bearer "+secret)
	check.Header.Set("X-Myco-Scope", "component-bootstrap")
	hc := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("component redirect refused") }}
	defer hc.CloseIdleConnections()
	response, err := hc.Do(check)
	if err != nil {
		return c.statusLocked(), errors.New("component_unavailable")
	}
	var actual struct {
		Protocol int    `json:"protocol"`
		Version  string `json:"version"`
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&actual)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || actual.Protocol != Protocol || actual.Version != selectedVersion || !CompatibleComponentVersion(actual.Version) {
		return c.statusLocked(), errors.New("incompatible_component")
	}
	c.process, c.input, c.done, c.address, c.secret, c.cleanup = cmd, stdin, done, h.Address, secret, cleanup
	failed = false
	return c.statusLocked(), nil
}
func (c *Component) Call(ctx context.Context, scope, method, path string, in, out any) error {
	c.mu.Lock()
	address, secret := c.address, c.secret
	c.mu.Unlock()
	if address == "" {
		return errors.New("component_not_running")
	}
	var b []byte
	var e error
	if in != nil {
		b, e = json.Marshal(in)
		if e != nil {
			return e
		}
	}
	timeout := 45 * time.Second
	if path == "/pick" {
		timeout = 150 * time.Second
	}
	if path == "/control/run" {
		timeout = 305 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, method, "http://"+address+path, bytes.NewReader(b))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("X-Myco-Scope", scope)
	req.Header.Set("Content-Type", "application/json")
	hc := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("component redirect refused") }}
	defer hc.CloseIdleConnections()
	if required := browserRunPreflight(method, path, b); len(required) != 0 {
		if err := requireBrowserFeatures(ctx, hc, address, secret, scope, required...); err != nil {
			return err
		}
	}
	res, e := hc.Do(req)
	if e != nil {
		return errors.New("component_unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		var v struct {
			Error     string `json:"error"`
			Code      string `json:"code"`
			Reason    string `json:"reason"`
			Applied   *int   `json:"applied"`
			Host      string `json:"host"`
			ErrorNode string `json:"error_node"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&v)
		if v.Code != "" || strings.HasPrefix(path, "/control/") && v.Error != "" {
			if v.Code == "" {
				v.Code = v.Error
			}
			return &ComponentError{Code: v.Code, Reason: v.Reason, Applied: v.Applied, Host: v.Host, ErrorNode: v.ErrorNode}
		}
		if v.Error == "" {
			v.Error = "component_failed"
		}
		if v.Error == "profile_locked" && v.Reason != "" {
			return &ProfileError{Reason: v.Reason}
		}
		return errors.New(v.Error)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 256<<10)).Decode(out)
	}
	return nil
}
func (c *Component) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}
func (c *Component) stopLocked() {
	if c.process == nil {
		return
	}
	_ = c.input.Close()
	select {
	case <-c.done:
	case <-time.After(componentStopGrace()):
		if c.cleanup != nil {
			c.cleanup()
		}
		_ = c.process.Process.Kill()
		<-c.done
	}
	if c.cleanup != nil {
		c.cleanup()
	}
	c.process = nil
	c.address = ""
	c.secret = ""
}
func (c *Component) Remove() error {
	if !filepath.IsAbs(c.Home) {
		return errors.New("home directory is unavailable")
	}
	c.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.installing {
		return ErrBusy
	}
	// Otherwise removing the current release makes an older installation
	// immediately appear installed again. Leave unknown versions untouched.
	var errs []error
	for _, version := range compatibleComponentVersions() {
		errs = append(errs, os.RemoveAll(filepath.Join(c.Home, ".myco", "components", "browser", version)))
	}
	return errors.Join(errs...)
}

var ErrBusy = errors.New("busy")

func nonce() string { return rand.Text() + rand.Text() }

// EnsureRunning never installs or upgrades a component. Only a trusted owner
// action may call Install; ordinary page calls can start an existing release.
func (c *Component) EnsureRunning(ctx context.Context) error {
	if c.Status().Running {
		return nil
	}
	if _, err := c.Start(ctx); err != nil {
		if c.Status().Running {
			return nil
		}
		return err
	}
	return nil
}
func (c *Component) Artifact(ctx context.Context, scope, id string) ([]byte, string, error) {
	if !regexp.MustCompile(`^a_[a-zA-Z0-9_-]{1,80}$`).MatchString(id) {
		return nil, "", errors.New("invalid_argument")
	}
	c.mu.Lock()
	address, secret := c.address, c.secret
	c.mu.Unlock()
	if address == "" {
		return nil, "", errors.New("component_not_running")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+address+"/artifacts/"+id, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("X-Myco-Scope", scope)
	hc := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("component redirect refused") }}
	defer hc.CloseIdleConnections()
	res, err := hc.Do(req)
	if err != nil {
		return nil, "", errors.New("component_unavailable")
	}
	defer res.Body.Close()
	kind := strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0])
	if res.StatusCode != 200 {
		return nil, "", errors.New("not_found")
	}
	if kind != "image/jpeg" && kind != "image/png" && kind != "image/webp" {
		return nil, "", errors.New("invalid_artifact")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
	if err != nil || len(data) > 4<<20 {
		return nil, "", errors.New("artifact_too_large")
	}
	return data, kind, nil
}

// ProfileError preserves the component's safe recovery reason without paths,
// process IDs, command lines, or other private process metadata.
type ProfileError struct{ Reason string }

func (e *ProfileError) Error() string { return "profile_locked" }

func browserEnvironmentKeys() []string {
	keys := []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "ProgramFiles", "ProgramFiles(x86)", "SystemRoot", "WINDIR", "TMP", "TEMP", "TMPDIR", "PATH", "LANG", "LC_ALL"}
	if runtime.GOOS == "linux" {
		keys = append(keys, "DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR")
	}
	return keys
}
