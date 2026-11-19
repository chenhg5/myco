package console

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// Connecting a computer from the App over SSH or WSL: log in and look
// around, run the workspace's join command there, wait for it to show up.
// It runs in the background; the App polls the job.

// ConnectStep is one step of connecting: login, install, online.
type ConnectStep struct {
	Key    string `json:"key"`
	State  string `json:"state"` // pending, running, done, failed
	Detail string `json:"detail,omitempty"`
}

// ConnectJob is one computer being connected.
type ConnectJob struct {
	ID    string        `json:"id"`
	Hub   string        `json:"hub"`
	Route Route         `json:"route"`
	Steps []ConnectStep `json:"steps"`
	// Found is what the login step saw there.
	Found map[string]string `json:"found,omitempty"`
	// Log is the last lines the install printed (the join link masked).
	Log       []string     `json:"log,omitempty"`
	Error     *RemoteError `json:"error,omitempty"`
	MachineID string       `json:"machine_id,omitempty"`
	Done      bool         `json:"done"`
}

type connectJobs struct {
	mu   sync.Mutex
	jobs map[string]*ConnectJob
}

func (j *connectJobs) put(c *ConnectJob) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.jobs == nil {
		j.jobs = map[string]*ConnectJob{}
	}
	j.jobs[c.ID] = c
}

func (j *connectJobs) update(id string, f func(*ConnectJob)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if c := j.jobs[id]; c != nil {
		f(c)
	}
}

func (j *connectJobs) get(id string) (ConnectJob, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	c := j.jobs[id]
	if c == nil {
		return ConnectJob{}, false
	}
	out := *c
	out.Steps = append([]ConnectStep(nil), c.Steps...)
	out.Log = append([]string(nil), c.Log...)
	return out, true
}

// probeScript looks around without changing anything; each finding is a
// "myco-probe <key> <value>" line.
const probeScript = `cd "$HOME" 2>/dev/null
p() { echo "myco-probe $1 $2"; }
p os "$(uname -s 2>/dev/null)"
p arch "$(uname -m 2>/dev/null)"
p host "$( (hostname || uname -n) 2>/dev/null | head -n1)"
command -v bash >/dev/null 2>&1 && p bash yes || p bash no
command -v curl >/dev/null 2>&1 && p curl yes || p curl no
M="$(command -v myco 2>/dev/null)"; [ -z "$M" ] && [ -x "$HOME/.local/bin/myco" ] && M="$HOME/.local/bin/myco"
[ -n "$M" ] && p myco "$("$M" --version 2>/dev/null | head -n1)"
[ -n "$WSL_DISTRO_NAME" ] && p wsl "$WSL_DISTRO_NAME"
p end ok
`

// installScript runs the join command with the link given to curl on its
// stdin (never on a command line), then names this computer's machine
// nodes so the App can tell which computer it became.
func installScript(joinURL string) string {
	q := "'" + strings.ReplaceAll(joinURL, "'", `'\''`) + "'"
	return `cd "$HOME" 2>/dev/null
S="$(printf 'url = "%s"\n' ` + q + ` | curl -fsSL --retry 3 -K - 2>&1)" || { echo "myco: could not fetch the join command: $S"; exit 3; }
printf '%s\n' "$S" | bash 2>&1 || exit $?
for f in "$HOME/.myco/machine.yaml"; do
  [ -f "$f" ] && sed -n 's/^ *name_prefix: *\(.*\)$/myco-node \1machine/p' "$f"
done
true
`
}

// parseProbe collects the "myco-probe" lines.
func parseProbe(lines []string) map[string]string {
	out := map[string]string{}
	for _, l := range lines {
		rest, ok := strings.CutPrefix(l, "myco-probe ")
		if !ok {
			continue
		}
		k, v, _ := strings.Cut(rest, " ")
		out[k] = strings.TrimSpace(v)
	}
	return out
}

// connectRoutes registers the SSH/WSL routes.
func (b *Backend) remoteRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/local/ssh-hosts", b.getSSHHosts)
	m.HandleFunc("GET /app/local/wsl", b.getWSL)
	m.HandleFunc("POST /app/hubs/{id}/machines/connect", b.withHub(b.postConnect))
	m.HandleFunc("GET /app/connect/{job}", func(w http.ResponseWriter, r *http.Request) {
		j, ok := b.connects.get(r.PathValue("job"))
		if !ok {
			writeErr(w, &client.Error{Status: http.StatusNotFound, Code: "not_found", Message: "no such connection"})
			return
		}
		writeJSON(w, http.StatusOK, j)
	})
}

// postConnect is POST /app/hubs/{id}/machines/connect {via, target, port}:
// starts connecting that computer; answers the job at once.
func (b *Backend) postConnect(w http.ResponseWriter, r *http.Request, c *client.Client) {
	var in Route
	if !decode(w, r, &in) {
		return
	}
	in.Target = strings.TrimSpace(in.Target)
	bad := in.Target == "" || strings.HasPrefix(in.Target, "-") || strings.ContainsAny(in.Target, " \t\r\n'\"`;|&$<>\\")
	if (in.Via != "ssh" && in.Via != "wsl") || bad || in.Port < 0 || in.Port > 65535 {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "say how to reach it: via ssh or wsl, and a plain name"})
		return
	}
	// The link is made now, while the request still carries the person's
	// rights on this workspace: only owners and admins may connect computers.
	res, err := c.CreateEnrollment(r.Context(), api.CreateEnrollmentRequest{Kind: "link", MaxUses: 1, TTLSeconds: 3600, Note: "myco app: over " + in.Via}, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	before := map[string]bool{}
	if ov, err := c.Overview(r.Context(), 0); err == nil {
		for _, m := range ov.Machines {
			before[m.ID] = true
		}
	}
	job := &ConnectJob{ID: newJobID(), Hub: r.PathValue("id"), Route: in, Steps: []ConnectStep{
		{Key: "login", State: "running"}, {Key: "install", State: "pending"}, {Key: "online", State: "pending"}}}
	b.connects.put(job)
	go b.runConnect(job.ID, in, res.JoinURL, c, before)
	j, _ := b.connects.get(job.ID)
	writeJSON(w, http.StatusOK, j)
}

func newJobID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "cj_" + hex.EncodeToString(b[:])
}

func (b *Backend) runConnect(id string, rt Route, joinURL string, c *client.Client, before map[string]bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	step := func(i int, state, detail string) {
		b.connects.update(id, func(j *ConnectJob) { j.Steps[i].State, j.Steps[i].Detail = state, detail })
	}
	fail := func(i int, e *RemoteError) {
		b.connects.update(id, func(j *ConnectJob) {
			j.Steps[i].State, j.Error, j.Done = "failed", e, true
		})
	}
	// 1. Log in and look around.
	var lines []string
	lctx, lcancel := context.WithTimeout(ctx, 40*time.Second)
	stderr, _, err := b.Local.runScript(lctx, rt, probeScript, func(s string) { lines = append(lines, s) })
	lcancel()
	found := parseProbe(lines)
	if err != nil {
		var re *RemoteError
		if !errors.As(err, &re) {
			re = &RemoteError{Code: "unreachable", Message: err.Error(), Command: rt.TerminalCommand()}
		}
		fail(0, re)
		return
	}
	if found["end"] != "ok" {
		fail(0, &RemoteError{Code: "no_shell", Message: orStr(lastLines(stderr, 3), "it did not answer like a Linux or macOS shell"), Command: rt.TerminalCommand()})
		return
	}
	b.connects.update(id, func(j *ConnectJob) { j.Found = found })
	if os := strings.ToLower(found["os"]); os != "linux" && os != "darwin" {
		fail(0, &RemoteError{Code: "unsupported", Message: "it runs " + orStr(found["os"], "an unknown system") + "; only Linux and macOS can be connected this way"})
		return
	}
	if found["curl"] != "yes" || found["bash"] != "yes" {
		miss := "curl"
		if found["bash"] != "yes" {
			miss = "bash"
		}
		fail(0, &RemoteError{Code: "missing_" + miss, Message: miss + " is not installed there"})
		return
	}
	step(0, "done", found["host"])

	// 2. Run the join command there.
	step(1, "running", "")
	var nodes []string
	_, code, err := b.Local.runScript(ctx, rt, installScript(joinURL), func(s string) {
		if n, ok := strings.CutPrefix(s, "myco-node "); ok {
			nodes = append(nodes, strings.TrimSpace(n))
			return
		}
		s = strings.ReplaceAll(s, joinURL, "<link>")
		b.connects.update(id, func(j *ConnectJob) {
			j.Log = append(j.Log, s)
			if len(j.Log) > 30 {
				j.Log = j.Log[len(j.Log)-30:]
			}
		})
	})
	if err != nil || code != 0 {
		var re *RemoteError
		if !errors.As(err, &re) {
			re = &RemoteError{Code: "install_failed", Message: "the install stopped", Command: rt.TerminalCommand()}
		}
		fail(1, re)
		return
	}
	step(1, "done", "")

	// 3. Wait for it on the workspace.
	step(2, "running", "")
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if mid := findConnected(ctx, c, before, nodes); mid != "" {
			b.connects.update(id, func(j *ConnectJob) {
				j.MachineID, j.Steps[2].State, j.Done = mid, "done", true
			})
			b.rememberReach(rt, j2hub(b, id), mid)
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	fail(2, &RemoteError{Code: "not_online", Message: "it was set up but has not shown up on this workspace", Command: rt.TerminalCommand()})
}

func j2hub(b *Backend, id string) string {
	j, _ := b.connects.get(id)
	return j.Hub
}

// findConnected is the computer that just connected: the one whose machine
// node is one of nodes, else one that was not there before.
func findConnected(ctx context.Context, c *client.Client, before map[string]bool, nodes []string) string {
	ov, err := c.Overview(ctx, 0)
	if err != nil {
		return ""
	}
	want := map[string]bool{}
	for _, n := range nodes {
		want[n] = true
	}
	for _, m := range ov.Machines {
		for _, a := range m.Agents {
			if want[a] && m.Online {
				return m.ID
			}
		}
	}
	for _, m := range ov.Machines {
		if !before[m.ID] && m.Online {
			return m.ID
		}
	}
	return ""
}

// ── how to reach each computer again ──

// reachPath keeps, per workspace and computer, how this App reaches it
// (names only: an SSH config name or user@host, a WSL distribution).
func (b *Backend) reachPath() string {
	if b.Store == nil || b.Store.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(b.Store.path), "reach.json")
}

var reachMu sync.Mutex

func (b *Backend) reaches() map[string]Route {
	out := map[string]Route{}
	if p := b.reachPath(); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(data, &out)
		}
	}
	return out
}

func (b *Backend) rememberReach(rt Route, hubID, machineID string) {
	p := b.reachPath()
	if p == "" || hubID == "" || machineID == "" {
		return
	}
	reachMu.Lock()
	defer reachMu.Unlock()
	all := b.reaches()
	all[hubID+"/"+machineID] = rt
	data, _ := json.MarshalIndent(all, "", "  ")
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	_ = os.WriteFile(p, data, 0o600)
}

func (b *Backend) reachOf(hubID, machineID string) (Route, bool) {
	reachMu.Lock()
	defer reachMu.Unlock()
	rt, ok := b.reaches()[hubID+"/"+machineID]
	return rt, ok
}
