package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/osutil"
	"github.com/chenhg5/myco/internal/release"
)

// Local runs a hub on this computer through the myco CLI (`myco hub up`),
// so the app can create a hub without a server. A GUI app starts with a
// minimal PATH, so the CLI and the tunnel tools are also looked for in the
// usual install locations.
type Local struct {
	// Home is the user's home directory (tests point it elsewhere).
	Home string
	// Run executes a command and returns its stdout (tests replace it).
	Run func(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
	// LookPath finds a program (tests replace it); default exec.LookPath.
	LookPath func(string) (string, error)
	// Install downloads the myco CLI into dir (tests replace it).
	Install func(ctx context.Context, dir string) (string, error)
	// Device names console tokens this app creates ("<device>-app").
	Device string
	// Dirs are searched after PATH (default: ~/.local/bin, Homebrew's and
	// /usr/local/bin).
	Dirs []string
	// Bundled is the myco CLI shipped inside the app ("" = none): used when
	// no other is installed, and installed into ~/.local/bin by EnsureCLI.
	Bundled string
}

// EnsureCLI installs the app's bundled myco CLI into ~/.local/bin when no
// myco is installed, so the local hub, local agents and the terminal all
// have one. It returns the installed path ("" if nothing was done).
func (l *Local) EnsureCLI() (string, error) {
	if l.Bundled == "" || l.find("myco", l.in(l.binDirs(), "myco")...) != "" {
		return "", nil
	}
	b, err := os.ReadFile(l.Bundled)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(l.Home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, exeName("myco"))
	return target, release.Replace(target, b)
}

func (l *Local) lookPath(name string) (string, error) {
	if l.LookPath != nil {
		return l.LookPath(name)
	}
	return exec.LookPath(name)
}

// find returns the first existing program among PATH and extra locations.
func (l *Local) find(name string, extra ...string) string {
	if p, err := l.lookPath(name); err == nil {
		return p
	}
	for _, p := range extra {
		// Windows has no executable bit: any file there counts.
		if st, err := os.Stat(p); err == nil && !st.IsDir() && (!osutil.UnixPermissions || st.Mode()&0o111 != 0) {
			return p
		}
	}
	return ""
}

func (l *Local) binDirs() []string {
	if l.Dirs != nil {
		return l.Dirs
	}
	return []string{filepath.Join(l.Home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin"}
}

func (l *Local) in(dirs []string, name string) []string {
	var out []string
	for _, d := range dirs {
		out = append(out, filepath.Join(d, exeName(name)))
	}
	return out
}

// exeName adds .exe on Windows.
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func (l *Local) mycoPath() string {
	if p := l.find("myco", l.in(l.binDirs(), "myco")...); p != "" {
		return p
	}
	if l.Bundled != "" {
		if _, err := os.Stat(l.Bundled); err == nil {
			return l.Bundled
		}
	}
	return ""
}

func (l *Local) cloudflaredPath() string {
	return l.find("cloudflared", l.in(l.binDirs(), "cloudflared")...)
}

func (l *Local) tailscalePath() string {
	return l.find("tailscale", append(l.in(l.binDirs(), "tailscale"),
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale")...)
}

// env is the environment for running myco: PATH extended with the
// directories of the tools it may need (myco hub up runs cloudflared or
// tailscale by name).
func (l *Local) env() []string {
	dirs := l.binDirs()
	for _, p := range []string{l.cloudflaredPath(), l.tailscalePath(), l.mycoPath()} {
		if p != "" {
			dirs = append([]string{filepath.Dir(p)}, dirs...)
		}
	}
	path := strings.Join(dirs, string(os.PathListSeparator))
	if cur := os.Getenv("PATH"); cur != "" {
		path += string(os.PathListSeparator) + cur
	}
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") && !strings.HasPrefix(kv, "MYCO_") {
			env = append(env, kv)
		}
	}
	return append(env, "PATH="+path, "MYCO_NO_UPDATE_CHECK=1")
}

func (l *Local) run(ctx context.Context, args ...string) ([]byte, error) {
	bin := l.mycoPath()
	if bin == "" {
		return nil, &client.Error{Status: http.StatusConflict, Code: "not_configured", Message: "the myco command line is not installed on this computer",
			Suggestion: "install it from this page first"}
	}
	runner := l.Run
	if runner == nil {
		runner = runCommand
	}
	out, err := runner(ctx, l.env(), bin, append(args, "--format", "json")...)
	if err != nil {
		return out, cliError(out, err)
	}
	return out, nil
}

func runCommand(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil && stdout.Len() == 0 {
		// myco prints its JSON error on stderr.
		return stderr.Bytes(), err
	}
	return stdout.Bytes(), err
}

// cliError turns a failed myco run into the API's error shape, keeping the
// CLI's own message and suggestion.
func cliError(out []byte, err error) error {
	var e struct{ Error, Message, Suggestion string }
	if i := bytes.IndexByte(out, '{'); i >= 0 && json.Unmarshal(out[i:], &e) == nil && e.Message != "" {
		return &client.Error{Status: http.StatusBadGateway, Code: orStr(e.Error, "cli_failed"), Message: e.Message, Suggestion: e.Suggestion}
	}
	return &client.Error{Status: http.StatusBadGateway, Code: "cli_failed", Message: strings.TrimSpace(err.Error() + " " + lastLine(out))}
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}

func (l *Local) hubEnv() map[string]string {
	vals, err := config.ReadEnvFile(filepath.Join(l.Home, ".myco", "hub", "hub.env"))
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, kv := range vals {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	return out
}

// Tool is an installed program.
type Tool struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
}

// LocalHub is the hub `myco hub up` runs on this computer.
type LocalHub struct {
	Running   bool   `json:"running"`
	LocalURL  string `json:"local_url"`
	PublicURL string `json:"public_url,omitempty"`
	Version   string `json:"version,omitempty"`
	Added     bool   `json:"added"`
}

// LocalStatus is GET /app/local.
type LocalStatus struct {
	Myco   *Tool            `json:"myco"`
	Hub    *LocalHub        `json:"hub"`
	Tools  map[string]*Tool `json:"tools"`
	Device string           `json:"device"`
}

func toolAt(p string) *Tool {
	if p == "" {
		return nil
	}
	return &Tool{Path: p}
}

func (b *Backend) localStatus(ctx context.Context) LocalStatus {
	l := b.Local
	st := LocalStatus{Tools: map[string]*Tool{"cloudflared": toolAt(l.cloudflaredPath()), "tailscale": toolAt(l.tailscalePath())}, Device: l.Device}
	if p := l.mycoPath(); p != "" {
		st.Myco = &Tool{Path: p}
		if out, err := l.run(ctx, "version", "--offline"); err == nil {
			var v struct {
				Version string `json:"version"`
				Client  string `json:"client"`
			}
			if json.Unmarshal(out, &v) == nil {
				st.Myco.Version = orStr(v.Version, v.Client)
			}
		}
	}
	env := l.hubEnv()
	if env == nil || env[config.EnvHub] == "" {
		return st
	}
	h := &LocalHub{LocalURL: env[config.EnvHub]}
	if c, err := client.New(h.LocalURL, ""); err == nil {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if hl, err := c.Health(cctx); err == nil {
			h.Running, h.PublicURL, h.Version = true, hl.PublicURL, hl.ServerVersion
		}
		cancel()
	}
	if hubs, err := b.Store.List(); err == nil {
		for _, x := range hubs {
			if x.URL == h.LocalURL || (h.PublicURL != "" && x.URL == strings.TrimRight(h.PublicURL, "/")) {
				h.Added = true
			}
		}
	}
	st.Hub = h
	return st
}

func (b *Backend) getLocal(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, b.localStatus(r.Context()))
}

// installMyco downloads the latest myco CLI (checked against the release's
// SHA256SUMS) into ~/.local/bin.
func (b *Backend) installMyco(w http.ResponseWriter, r *http.Request) {
	l := b.Local
	if p := l.mycoPath(); p != "" {
		writeJSON(w, http.StatusOK, Tool{Path: p})
		return
	}
	dir := filepath.Join(l.Home, ".local", "bin")
	install := l.Install
	if install == nil {
		install = installLatest
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	p, err := install(ctx, dir)
	if err != nil {
		writeErr(w, &client.Error{Status: http.StatusBadGateway, Code: "install_failed", Message: err.Error(),
			Suggestion: "check the network, or install it in a terminal: curl -fsSL https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-client.sh | bash"})
		return
	}
	writeJSON(w, http.StatusOK, Tool{Path: p})
}

func installLatest(ctx context.Context, dir string) (string, error) {
	hc := &http.Client{Timeout: 30 * time.Minute} // slow links: 15 MB at 40 KB/s is ~7 minutes
	tag, err := release.Latest(ctx, hc, release.APIURL(os.Getenv))
	if err != nil {
		return "", err
	}
	bin, err := release.Fetch(ctx, hc, release.DownloadURL(os.Getenv), tag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, exeName("myco"))
	return target, release.Replace(target, bin)
}

// StartLocalResult is POST /app/local/hub.
type StartLocalResult struct {
	Hub       HubStatus `json:"hub"`
	PublicURL string    `json:"public_url"`
	JoinURL   string    `json:"join_url,omitempty"`
	Warnings  []string  `json:"warnings"`
}

// startLocal runs `myco hub up`, issues a console token with the local
// admin token (the app runs as the same user, so `myco` finds hub.env), and
// adds the hub to the app by its local address.
func (b *Backend) startLocal(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Tunnel   string `json:"tunnel"`
		Hostname string `json:"hostname"`
	}
	if !decode(w, r, &in) {
		return
	}
	switch in.Tunnel {
	case "cloudflare", "tailscale", "none":
	default:
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "tunnel must be cloudflare, tailscale or none"})
		return
	}
	in.Hostname = strings.TrimSpace(in.Hostname)
	if in.Hostname != "" && in.Tunnel != "cloudflare" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "a hostname is only used with the cloudflare tunnel"})
		return
	}
	l := b.Local
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	args := []string{"hub", "up", "--tunnel", in.Tunnel}
	if in.Hostname != "" {
		args = append(args, "--hostname", in.Hostname)
	}
	out, err := l.run(ctx, args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	var up struct {
		PublicURL     string `json:"public_url"`
		Reachable     bool   `json:"reachable"`
		JoinURL       string `json:"join_url"`
		JoinLinkError string `json:"join_link_error"`
	}
	if err := json.Unmarshal(out, &up); err != nil {
		writeErr(w, fmt.Errorf("unexpected output from myco hub up: %w", err))
		return
	}
	res := StartLocalResult{PublicURL: up.PublicURL, JoinURL: up.JoinURL, Warnings: []string{}}
	if in.Tunnel == "cloudflare" && in.Hostname == "" {
		res.Warnings = append(res.Warnings, "这是 Cloudflare 临时地址：隧道重启后地址会变，已加入的机器和链接会失效。长期使用请换成自己的域名或 Tailscale。")
	}
	if in.Tunnel != "none" && !up.Reachable {
		res.Warnings = append(res.Warnings, "公网地址暂时还访问不到（DNS 可能需要几分钟生效），本机使用不受影响。")
	}
	if up.JoinLinkError != "" {
		res.Warnings = append(res.Warnings, "入网链接没有生成："+up.JoinLinkError)
	}

	env := l.hubEnv()
	localURL := env[config.EnvHub]
	if localURL == "" {
		writeErr(w, errors.New("myco hub up finished but ~/.myco/hub/hub.env has no hub address"))
		return
	}
	// Already in the app: nothing more to add.
	if hubs, err := b.Store.List(); err == nil {
		for _, h := range hubs {
			if h.URL == localURL {
				res.Hub = HubStatus{Hub: h, OK: true}
				writeJSON(w, http.StatusOK, res)
				return
			}
		}
	}
	tokOut, err := l.run(ctx, "console", "token", "create", "--name", orStr(l.Device, "this-mac")+"-app", "--hub", localURL)
	if err != nil {
		writeErr(w, err)
		return
	}
	var tok struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(tokOut, &tok) != nil || tok.Token == "" {
		writeErr(w, errors.New("myco console token create did not return a token"))
		return
	}
	h, err := b.Store.Add("本机 Hub", localURL)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := b.Secrets.Set(h.ID, tok.Token); err != nil {
		_, _ = b.Store.Remove(h.ID)
		writeErr(w, fmt.Errorf("cannot save the token in the keychain: %w", err))
		return
	}
	res.Hub = HubStatus{Hub: h, OK: true}
	writeJSON(w, http.StatusCreated, res)
}

func (b *Backend) stopLocal(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	if _, err := b.Local.run(ctx, "hub", "down"); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopped": true})
}
