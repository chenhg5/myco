package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

type targetInstall struct {
	sid        proto.TargetSessionID
	ready      chan struct{}
	configured chan struct{}
	err        error
}

type session struct {
	commandCtx          context.Context
	commandToken        uint64
	headless            bool
	seenTargets         map[proto.TargetTargetID]bool
	sequence            uint64
	observations        []observation
	observed            chan struct{}
	dialogs             map[proto.TargetSessionID]string
	downloadNames       map[string]string
	picking             chan picked
	ready               chan struct{}
	closed              bool
	installs            map[proto.TargetTargetID]*targetInstall
	e                   *engine
	id, scope           string
	ctx                 context.Context
	cancel              context.CancelFunc
	browser             *rod.Browser
	cmd                 *exec.Cmd
	done                chan struct{}
	lock                *os.File
	guard               func()
	once                sync.Once
	mu                  sync.Mutex
	pages               map[proto.TargetTargetID]*rod.Page
	current             proto.TargetTargetID
	controller          string
	recorder            *recording
	frames              map[proto.PageFrameID]frameInfo
	contexts            map[string]map[proto.RuntimeExecutionContextID]proto.PageFrameID
	targets             map[proto.TargetSessionID]proto.TargetTargetID
	rendererNavigations map[proto.PageFrameID]bool
}
type frameInfo struct {
	Parent   proto.PageFrameID
	Selector string
	URL      string
}

func installedBrowsers() map[string]string {
	out := map[string]string{}
	add := func(name string, paths ...string) {
		for _, p := range paths {
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
				out[name] = p
				return
			}
		}
	}
	switch runtime.GOOS {
	case "darwin":
		add("chrome", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if base != "" {
				add("chrome", filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
			}
		}
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
			if base != "" {
				add("edge", filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
			}
		}
	default:
		for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
			if p, err := exec.LookPath(name); err == nil {
				out["chrome"] = p
				break
			}
		}
	}
	return out
}
func checkedURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("invalid_argument")
	}
	return nil
}
func (e *engine) openSession(ctx context.Context, owner, id string, headless bool) (*session, error) {
	e.mu.Lock()
	if old := e.sessions[id]; old != nil {
		e.mu.Unlock()
		select {
		case <-old.ready:
		default:
			return nil, errors.New("busy")
		}
		if old.ctx.Err() != nil {
			return nil, errors.New("busy")
		}
		if old.scope != owner || old.headless != headless {
			return nil, errors.New("busy")
		}
		return old, nil
	}
	if _, ok := e.profiles[id]; !ok || !idPattern.MatchString(id) {
		e.mu.Unlock()
		return nil, errors.New("not_found")
	}
	browsers := installedBrowsers()
	binary := browsers["chrome"]
	if binary == "" {
		binary = browsers["edge"]
	}
	if binary == "" {
		e.mu.Unlock()
		return nil, errors.New("browser_missing")
	}
	dir := filepath.Join(e.dir, "profiles", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if actual, err := filepath.EvalSymlinks(dir); err != nil || actual != dir {
		e.mu.Unlock()
		return nil, errors.New("invalid_argument")
	}
	lock, err := lockProfile(filepath.Join(dir, ".myco-lease"))
	if err != nil {
		e.mu.Unlock()
		return nil, errors.New("profile_locked")
	}
	run, cancel := context.WithCancel(e.ctx)
	s := &session{headless: headless, ready: make(chan struct{}), installs: map[proto.TargetTargetID]*targetInstall{}, e: e, id: id, scope: owner, ctx: run, cancel: cancel, done: make(chan struct{}), lock: lock, pages: map[proto.TargetTargetID]*rod.Page{}, frames: map[proto.PageFrameID]frameInfo{}, contexts: map[string]map[proto.RuntimeExecutionContextID]proto.PageFrameID{}, targets: map[proto.TargetSessionID]proto.TargetTargetID{}}
	e.sessions[id] = s // Reserve the lease before slow process/CDP startup.
	e.mu.Unlock()
	failed := true
	defer close(s.ready)
	defer func() {
		if failed {
			s.close()
		}
	}()
	// Never reuse a stale DevTools endpoint: it might belong to an old browser.
	_ = os.Remove(filepath.Join(dir, "DevToolsActivePort"))
	args := []string{"--user-data-dir=" + dir, "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1", "--no-first-run", "--no-default-browser-check", "--disable-sync", "--disable-blink-features=AutomationControlled", "--window-size=1280,900", "about:blank"}
	if headless {
		args = append([]string{"--headless=new"}, args...)
	}
	cmd := exec.Command(binary, args...)
	for _, key := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "ProgramFiles", "ProgramFiles(x86)", "SystemRoot", "WINDIR", "TMP", "TEMP", "TMPDIR", "PATH", "LANG", "LC_ALL"} {
		if value := os.Getenv(key); value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	// Environment is inherited from the component, whose App launcher keeps
	// only OS paths and locale. No Hub/provider credentials reach Chrome.
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, context.Canceled
	}
	s.trace("stage:process-start", "", false)
	if err = cmd.Start(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.trace("stage:process-start", "", true)
	s.cmd = cmd
	s.mu.Unlock()
	go func() { _ = cmd.Wait(); close(s.done); s.cancel() }()
	guard, err := guardBrowser(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		<-s.done
		return nil, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		guard()
		return nil, context.Canceled
	}
	s.guard = guard
	s.mu.Unlock()
	startup, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()
	s.trace("stage:DevTools-port", "", false)
	endpoint := ""
	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()
	for endpoint == "" {
		raw, _ := os.ReadFile(filepath.Join(dir, "DevToolsActivePort"))
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if len(lines) == 2 {
			port, err := strconv.Atoi(lines[0])
			if err == nil && port > 0 && port < 65536 && strings.HasPrefix(lines[1], "/devtools/browser/") {
				endpoint = "ws://" + net.JoinHostPort("127.0.0.1", lines[0]) + lines[1]
				break
			}
		}
		select {
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		case <-startup.Done():
			return nil, startup.Err()
		case <-s.done:
			return nil, errors.New("browser_failed")
		case <-tick.C:
		}
	}
	s.trace("stage:DevTools-port", "", true)
	ws := &cdp.WebSocket{}
	if err = ws.Connect(run, endpoint, nil); err != nil {
		return nil, err
	}
	wire := cdp.New().Logger(quietLog{}).Start(ws)
	b := rod.New().Context(run).ControlURL("").Client(&sessionClient{base: wire, owner: s}).NoDefaultDevice().Trace(false).SlowMotion(0).Logger(quietLog{})
	s.trace("stage:Rod-connect", "", false)
	if err = b.Connect(); err != nil {
		return nil, err
	}
	s.trace("stage:Rod-connect", "", true)
	s.browser = b
	if err = s.attach(); err != nil {
		return nil, err
	}
	// Do not let Pages() race auto-attachment by attaching all targets a
	// second time. Each primary installer constructs its Rod Page itself.
	s.trace("stage:Rod-pages", "", false)
	targets, err := (proto.TargetGetTargets{}).Call(b)
	if err != nil {
		return nil, err
	}
	for _, info := range targets.TargetInfos {
		if info.Type != "page" {
			continue
		}
		if _, err = s.waitPage(startup, info.TargetID); err != nil {
			return nil, err
		}
	}
	s.trace("stage:Rod-pages", "", true)
	failed = false
	go func() { <-run.Done(); s.close() }()
	return s, nil
}
func (s *session) close() {
	s.once.Do(func() {
		s.cancel()
		s.mu.Lock()
		s.closed = true
		rec := s.recorder
		cmd := s.cmd
		guard := s.guard
		s.mu.Unlock()
		if rec != nil {
			rec.finish()
		}
		if cmd != nil {
			if guard != nil {
				guard()
			} else {
				_ = cmd.Process.Kill()
			}
			select {
			case <-s.done:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
				<-s.done
			}
		}
		if s.lock != nil {
			_ = s.lock.Close()
		}
		s.e.mu.Lock()
		if s.e.sessions[s.id] == s {
			delete(s.e.sessions, s.id)
		}
		s.e.mu.Unlock()
	})
}
func (e *engine) session(owner, id string) (*session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.sessions[id]
	if s == nil || s.scope != owner {
		return nil, errors.New("not_found")
	}
	select {
	case <-s.ctx.Done():
		return nil, errors.New("not_found")
	default:
	}
	return s, nil
}
func (s *session) claim(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controller != "" {
		return errors.New("busy")
	}
	s.controller = id
	return nil
}
func (s *session) release(id string) {
	s.mu.Lock()
	if s.controller == id {
		s.controller = ""
	}
	s.mu.Unlock()
}
func (s *session) page() (*rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pages[s.current]
	if p == nil {
		return nil, errors.New("target_not_found")
	}
	return p, nil
}

type quietLog struct{}

func (quietLog) Println(...interface{}) {}

// Rod sometimes routes frame/repaint helpers through its original root Page,
// losing a derived Page's context. Carry the active operation to the wire too.
func (s *session) commands(ctx context.Context) func() {
	s.mu.Lock()
	s.commandToken++
	token := s.commandToken
	s.commandCtx = ctx
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		if s.commandToken == token {
			s.commandCtx = nil
		}
		s.mu.Unlock()
	}
}
