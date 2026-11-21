package remotedesktop

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
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
	"runtime"
	"strings"
	"sync"
	"time"
)

const ComponentVersion = "0.2.0"
const ComponentRelease = "https://github.com/chenhg5/myco/releases/download/remote-v" + ComponentVersion

// Component owns only the optional process and its private directory. A new
// engine can implement the same HTTP protocol without changing the App or Hub.
type Component struct {
	Home                     string
	HTTP                     *http.Client
	mu                       sync.Mutex
	installing               bool
	process                  *exec.Cmd
	input                    io.WriteCloser
	done                     chan struct{}
	cleanup                  func()
	address, secret, pairing string
}

func (c *Component) dir() string {
	return filepath.Join(c.Home, ".myco", "components", "remote", ComponentVersion)
}
func program(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
func remoteSupported() bool {
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
			c.pairing = ""
		default:
		}
	}
	_, a := os.Stat(filepath.Join(c.dir(), program("myco-remote")))
	_, b := os.Stat(filepath.Join(c.dir(), program("ffmpeg")))
	s := ComponentStatus{Protocol: Protocol, Version: ComponentVersion, Installed: a == nil && b == nil, Supported: remoteSupported(), Running: c.process != nil}
	if s.Running {
		s.PairingCode = c.pairing
	}
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
	if !filepath.IsAbs(c.Home) {
		return errors.New("home directory is unavailable")
	}
	c.mu.Lock()
	if c.installing || c.process != nil {
		c.mu.Unlock()
		return ErrBusy
	}
	c.installing = true
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.installing = false; c.mu.Unlock() }()
	if !remoteSupported() {
		return errors.New("unsupported_platform")
	}
	name := "myco-remote_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
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
			return nil, fmt.Errorf("component download unavailable (HTTP %d); use a desktop build with published remote components", res.StatusCode)
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
	if c.process != nil {
		return ErrBusy
	}
	if _, e = os.Stat(c.dir()); e == nil {
		return nil
	}
	return os.Rename(tmp, c.dir())
}
func extractComponent(data []byte, dir string) error {
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return e
	}
	required := map[string]bool{program("myco-remote"): false, program("ffmpeg"): false}
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
func (c *Component) Start(ctx context.Context, control bool, ice json.RawMessage) (ComponentStatus, error) {
	if !filepath.IsAbs(c.Home) {
		return ComponentStatus{}, errors.New("home directory is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process != nil {
		return c.statusLocked(), ErrBusy
	}
	if !c.statusLocked().Installed {
		return c.statusLocked(), errors.New("component_not_installed")
	}
	secret := nonce()
	raw := nonce()
	pair := raw[:8] // 32 bits; five failures pause pairing for a minute.
	cfg := map[string]any{"secret": secret, "pairing_code": pair, "control": control}
	if len(ice) > 0 {
		cfg["ice_servers"] = ice
	}
	// The component is a child with no Hub credentials and no auto-start service.
	cmd := exec.Command(filepath.Join(c.dir(), program("myco-remote")))
	hideComponent(cmd)
	for _, key := range []string{"HOME", "USERPROFILE", "SystemRoot", "WINDIR", "TMP", "TEMP", "TMPDIR", "PATH", "LANG", "LC_ALL"} {
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
	cleanup, e := guardComponent(cmd)
	if e != nil {
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return c.statusLocked(), e
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
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
		Address  string `json:"address"`
		Protocol int    `json:"protocol"`
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
	host, _, e := net.SplitHostPort(h.Address)
	if e != nil || host != "127.0.0.1" || h.Protocol != Protocol {
		return c.statusLocked(), errors.New("incompatible_component")
	}
	c.process, c.input, c.done, c.address, c.secret, c.pairing, c.cleanup = cmd, stdin, done, h.Address, secret, pair, cleanup
	failed = false
	return c.statusLocked(), nil
}
func (c *Component) Call(ctx context.Context, method, path string, in, out any) error {
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
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, method, "http://"+address+path, bytes.NewReader(b))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	hc := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("component redirect refused") }}
	defer hc.CloseIdleConnections()
	res, e := hc.Do(req)
	if e != nil {
		return errors.New("component_unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		var v struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&v)
		if v.Error == "" {
			v.Error = "component_failed"
		}
		return errors.New(v.Error)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 70<<10)).Decode(out)
	}
	return nil
}
func (c *Component) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process == nil {
		return
	}
	_ = c.input.Close()
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
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
	c.pairing = ""
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
	return os.RemoveAll(c.dir())
}
