// Package integration exercises a real hub over HTTP: SQLite on disk, the
// HTTP API, the Go client and the CLI (including `runner start` with the
// shell and exec harnesses), all in one process. Unlike the unit tests, the
// pieces are only wired together the way `myco hub serve` and the CLI wire
// them, so these tests pin down behaviour across layers: restarts, leases,
// concurrent pulls, allow-lists, broadcasts and attachments.
package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/chenhg5/myco/internal/release"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/cli"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/hubserver"
	"github.com/chenhg5/myco/internal/infra/sqlite"
	"github.com/chenhg5/myco/internal/osutil"
	"github.com/chenhg5/myco/internal/testutil"
)

const adminToken = "integration-admin-token-0123456789"

// hub is one running hub process (handler + HTTP server) on a DB file.
type hub struct {
	t   *testing.T
	db  string
	url string
	srv *httptest.Server
	h   *hubserver.Hub
}

func startHub(t *testing.T, db string, p app.MessagePolicy) *hub {
	t.Helper()
	h, err := hubserver.Build(hubserver.Options{DBPath: db, AdminToken: adminToken, Policy: p,
		OnlineWindow: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler)
	x := &hub{t: t, db: db, url: srv.URL, srv: srv, h: h}
	t.Cleanup(x.stop)
	return x
}

// stop shuts the hub down (idempotent), like killing `myco hub serve`.
func (x *hub) stop() {
	if x.srv != nil {
		x.srv.Close()
		x.h.Close()
		x.srv = nil
	}
}

func (x *hub) admin() *client.Client { return x.client(adminToken) }

func (x *hub) client(token string) *client.Client {
	x.t.Helper()
	c, err := client.New(x.url, token)
	if err != nil {
		x.t.Fatal(err)
	}
	return c
}

// register creates an agent and returns its token.
func (x *hub) register(req api.RegisterRequest) string {
	x.t.Helper()
	out, err := x.admin().Register(context.Background(), req, false)
	if err != nil {
		x.t.Fatalf("register %s: %v", req.Name, err)
	}
	return out.Token
}

func policy(lease time.Duration, attempts int) app.MessagePolicy {
	p := app.DefaultMessagePolicy()
	p.Lease, p.MaxAttempts = lease, attempts
	return p
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return c
}

// A hub restart keeps agents, tokens and queued work; a message that was
// pulled but never acknowledged before the crash is delivered again.
func TestRestartKeepsStateAndRedeliversUnacked(t *testing.T) {
	c := ctx(t)
	db := filepath.Join(t.TempDir(), "hub.db")
	h1 := startHub(t, db, policy(300*time.Millisecond, 5))
	aliceTok := h1.register(api.RegisterRequest{Name: "alice"})
	bobTok := h1.register(api.RegisterRequest{Name: "bob"})

	sent, err := h1.client(aliceTok).Send(c, api.SendRequest{To: "bob", Body: "survive the restart"}, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h1.client(bobTok).Pull(c, 10, 0)
	if err != nil || len(got) != 1 || got[0].ID != sent.Message.ID {
		t.Fatalf("first pull: %v %+v", err, got)
	}
	h1.stop() // crash before bob acks

	h2 := startHub(t, db, policy(300*time.Millisecond, 5))
	bob := h2.client(bobTok)
	if _, err := bob.Me(c); err != nil {
		t.Fatalf("token lost across restart: %v", err)
	}
	if got, _ := bob.Pull(c, 10, 0); len(got) != 0 {
		t.Fatalf("redelivered before the lease expired: %+v", got)
	}
	time.Sleep(350 * time.Millisecond)
	got, err = bob.Pull(c, 10, 0)
	if err != nil || len(got) != 1 || got[0].ID != sent.Message.ID || got[0].Attempts != 2 {
		t.Fatalf("redelivery after restart: %v %+v", err, got)
	}
	if _, err := bob.Reply(c, got[0].ID, "made it", false); err != nil {
		t.Fatal(err)
	}
	reply, err := h2.client(aliceTok).WaitReply(c, sent.Message.ID, 5*time.Second)
	if err != nil || reply == nil || reply.Body != "made it" {
		t.Fatalf("reply after restart: %v %+v", err, reply)
	}
}

// Many concurrent long-polls on one inbox never hand the same message out twice.
func TestConcurrentPullsNeverDoubleDeliver(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	sender := h.client(h.register(api.RegisterRequest{Name: "sender"}))
	workerTok := h.register(api.RegisterRequest{Name: "worker"})
	const n = 60
	for i := 0; i < n; i++ {
		if _, err := sender.Send(c, api.SendRequest{To: "worker", Type: "note", Body: fmt.Sprint(i)}, false); err != nil {
			t.Fatal(err)
		}
	}
	var (
		mu   sync.Mutex
		seen = map[string]int{}
		wg   sync.WaitGroup
	)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := h.client(workerTok)
			for {
				msgs, err := w.Pull(c, 3, 200*time.Millisecond)
				if err != nil {
					t.Error(err)
					return
				}
				if len(msgs) == 0 {
					return
				}
				mu.Lock()
				for _, m := range msgs {
					seen[m.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Fatalf("got %d distinct messages, want %d", len(seen), n)
	}
	for id, k := range seen {
		if k != 1 {
			t.Errorf("%s delivered %d times", id, k)
		}
	}
}

// A message that keeps being pulled without an ack is failed after
// MaxAttempts deliveries instead of bouncing forever.
func TestUnackedMessageFailsAfterMaxAttempts(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(80*time.Millisecond, 2))
	alice := h.client(h.register(api.RegisterRequest{Name: "alice"}))
	bob := h.client(h.register(api.RegisterRequest{Name: "bob"}))
	sent, err := alice.Send(c, api.SendRequest{To: "bob", Body: "never acked"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		got, err := bob.Pull(c, 1, 0)
		if err != nil || len(got) != 1 || got[0].Attempts != attempt {
			t.Fatalf("attempt %d: %v %+v", attempt, err, got)
		}
		time.Sleep(120 * time.Millisecond)
	}
	if got, _ := bob.Pull(c, 1, 0); len(got) != 0 {
		t.Fatalf("delivered a third time: %+v", got)
	}
	m, err := alice.GetMessage(c, sent.Message.ID)
	if err != nil || m.Status != "failed" {
		t.Fatalf("status after exhausting attempts: %v %+v", err, m)
	}
}

// Broadcast fans out by label selector and tracks every reply.
func TestBroadcastFansOutAndCollectsReplies(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	lead := h.client(h.register(api.RegisterRequest{Name: "lead"}))
	toks := map[string]string{}
	for _, n := range []string{"r1", "r2", "r3"} {
		toks[n] = h.register(api.RegisterRequest{Name: n, Labels: []string{"group=review"}})
	}
	h.register(api.RegisterRequest{Name: "other", Labels: []string{"group=ops"}})

	bc, err := lead.Broadcast(c, api.BroadcastRequest{Selector: "group=review", Body: "status?"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for n, tok := range toks {
		w := h.client(tok)
		msgs, err := w.Pull(c, 5, 0)
		if err != nil || len(msgs) != 1 || msgs[0].BroadcastID != bc.BroadcastID {
			t.Fatalf("%s: %v %+v", n, err, msgs)
		}
		if _, err := w.Reply(c, msgs[0].ID, "ok from "+n, false); err != nil {
			t.Fatal(err)
		}
	}
	if msgs, _ := h.client(h.register(api.RegisterRequest{Name: "other", RotateToken: true})).Pull(c, 5, 0); len(msgs) != 0 {
		t.Fatalf("agent outside the selector got the broadcast: %+v", msgs)
	}
	st, err := lead.BroadcastStatus(c, bc.BroadcastID, false)
	if err != nil || st.Total != 3 || st.Answered != 3 {
		t.Fatalf("broadcast status: %v %+v", err, st)
	}
}

// Attachments: the recipient downloads exactly the uploaded bytes; an agent
// that is neither uploader nor recipient cannot.
func TestAttachmentsRoundTripAndStayPrivate(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	alice := h.client(h.register(api.RegisterRequest{Name: "alice"}))
	bob := h.client(h.register(api.RegisterRequest{Name: "bob"}))
	eve := h.client(h.register(api.RegisterRequest{Name: "eve"}))

	dir := t.TempDir()
	content := bytes.Repeat([]byte("myco attachment\n"), 4096)
	src := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := alice.UploadFile(c, src)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if f.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("hub sha256 %s", f.SHA256)
	}
	if _, err := alice.Send(c, api.SendRequest{To: "bob", Type: "note", Body: "see attached", Attachments: []string{f.ID}}, false); err != nil {
		t.Fatal(err)
	}
	got, path, err := bob.DownloadFile(c, f.ID, filepath.Join(dir, "bob"))
	if err != nil || got.ID != f.ID {
		t.Fatalf("bob download: %v", err)
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, content) {
		t.Fatalf("downloaded %d bytes, want %d identical", len(b), len(content))
	}
	_, _, err = eve.DownloadFile(c, f.ID, filepath.Join(dir, "eve"))
	if e, ok := client.AsError(err); !ok || (e.Code != "permission_denied" && e.Code != "not_found") {
		t.Fatalf("eve downloaded someone else's attachment: %v", err)
	}
}

// --- CLI-level tests: the same entry point as the binary -------------------

// cliEnv runs `myco` in-process as one identity.
type cliEnv struct {
	t   *testing.T
	env map[string]string
}

func newCLI(t *testing.T, h *hub, cfg string) *cliEnv {
	return &cliEnv{t: t, env: map[string]string{"MYCO_HUB": h.url, "MYCO_CONFIG": cfg, "MYCO_ADMIN_TOKEN": adminToken,
		"MYCO_FORMAT": "json", "PATH": os.Getenv("PATH"), "HOME": os.Getenv("HOME")}}
}

func (e *cliEnv) run(c context.Context, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := cli.Run(c, args, strings.NewReader(""), &out, &errb, func(k string) string { return e.env[k] }, false)
	return code, out.String(), errb.String()
}

func (e *cliEnv) must(c context.Context, args ...string) string {
	e.t.Helper()
	code, out, errb := e.run(c, args...)
	if code != 0 {
		e.t.Fatalf("myco %s: exit %d\n%s%s", strings.Join(args, " "), code, out, errb)
	}
	return out
}

// isolatedHome points HOME at a temp dir: the runner keeps state, sessions
// and attachments under ~/.myco.
func isolatedHome(t *testing.T) string {
	home := t.TempDir()
	testutil.SetHome(t, home)
	return home
}

// A shell-harness runner turns a task into a reply: the full path a real
// agent takes (pull, lease, ack, run the CLI, reply) with a deterministic CLI.
func TestRunnerAnswersTasksThroughShellHarness(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	alice := newCLI(t, h, filepath.Join(home, "alice.yaml"))
	worker := newCLI(t, h, filepath.Join(home, "worker.yaml"))
	alice.must(c, "agent", "register", "--name", "alice")
	worker.must(c, "agent", "register", "--name", "worker", "--harness", "shell")

	out := alice.must(c, "message", "send", "--to", "worker", "--body", "make me loud")
	var sent api.SendResponse
	if err := json.Unmarshal([]byte(out), &sent); err != nil {
		t.Fatal(err)
	}
	// The prompt arrives on stdin; answer with the upper-cased message body.
	body := `sed -n '/^--- message body ---$/,$p' | tail -n +2 | tr a-z A-Z`
	worker.must(c, "runner", "start", "--once", "--harness", "shell", "--command", body, "--workdir", t.TempDir())

	out = alice.must(c, "message", "wait", "--id", sent.Message.ID, "--timeout", "5s")
	if !strings.Contains(out, "MAKE ME LOUD") {
		t.Fatalf("reply: %s", out)
	}
	// The worker's runner is bound to its session; the hub now knows the harness.
	out = alice.must(c, "agent", "get", "--name", "worker")
	if !strings.Contains(out, `"harness": "shell"`) {
		t.Fatalf("binding not recorded: %s", out)
	}
}

// An exec node runs commands only for allowed senders, only from its
// allow-list, and always answers with a structured result.
func TestExecNodeEnforcesSendersAndAllowList(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	lead := newCLI(t, h, filepath.Join(home, "lead.yaml"))
	coder := newCLI(t, h, filepath.Join(home, "coder.yaml"))
	ciCfg := filepath.Join(home, "ci.yaml")
	ci := newCLI(t, h, ciCfg)
	lead.must(c, "agent", "register", "--name", "lead")
	coder.must(c, "agent", "register", "--name", "coder")
	ci.must(c, "agent", "register", "--name", "ci", "--harness", "exec", "--allow-from", "lead", "--workdir", t.TempDir())
	f, err := os.OpenFile(ciCfg, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(f, "runner:\n  harness: exec\n  exec:\n    allow: [\"echo *\"]\n")
	f.Close()

	// Senders outside allow_from are refused by the hub before anything runs.
	if code, _, _ := coder.run(c, "exec", "--to", "ci", "--cmd", "echo hi"); code != cli.ExitPermission {
		t.Fatalf("coder exec: exit %d, want %d", code, cli.ExitPermission)
	}

	// lead's exec blocks until the node answers; serve the node meanwhile.
	type res struct {
		code int
		out  string
	}
	run := func(cmd string) res {
		done := make(chan res, 1)
		go func() {
			code, out, _ := lead.run(c, "exec", "--to", "ci", "--cmd", cmd, "--timeout", "10s")
			done <- res{code, out}
		}()
		for {
			select {
			case r := <-done:
				return r
			case <-time.After(100 * time.Millisecond):
				ci.run(c, "runner", "start", "--once")
			}
		}
	}
	if r := run("echo 'hello from ci'"); r.code != 0 || !strings.Contains(r.out, "hello from ci") {
		t.Fatalf("allowed command: exit %d %s", r.code, r.out)
	}
	r := run("cat /etc/hostname")
	if r.code == 0 || !strings.Contains(r.out, `"denied": true`) {
		t.Fatalf("command outside the allow-list ran: exit %d %s", r.code, r.out)
	}
	// Chaining is rejected even when the prefix matches.
	if r := run("echo ok; cat /etc/hostname"); r.code == 0 || !strings.Contains(r.out, `"denied": true`) {
		t.Fatalf("chained command ran: exit %d %s", r.code, r.out)
	}
	// Every decision is audited on the node.
	audit, err := os.ReadFile(filepath.Join(home, ".myco", "logs", "ci.exec.jsonl"))
	if err != nil || strings.Count(string(audit), "\n") < 3 {
		t.Fatalf("audit log: %v\n%s", err, audit)
	}
}

// A hub that stopped halfway through removing an agent (older hubs deleted
// the agent first) left an alias pointing at nothing; a new agent with the
// old name would have inherited it. The hub sweeps such aliases at start-up.
func TestRestartSweepsDanglingAliases(t *testing.T) {
	c := ctx(t)
	db := filepath.Join(t.TempDir(), "hub.db")
	h1 := startHub(t, db, app.DefaultMessagePolicy())
	h1.register(api.RegisterRequest{Name: "bob"})
	if _, err := h1.admin().SetAlias(c, "reviewer", "bob", false); err != nil {
		t.Fatal(err)
	}
	h1.stop()

	// Simulate the interrupted removal: the agent row is gone, the alias is not.
	raw, err := sqlite.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewAgentRepo(raw).Delete(c, "bob"); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	h2 := startHub(t, db, app.DefaultMessagePolicy())
	if list, err := h2.admin().ListAliases(c); err != nil || len(list) != 0 {
		t.Fatalf("dangling alias survived a restart: %v %+v", err, list)
	}
	// A new "bob" does not pick up the old alias.
	h2.register(api.RegisterRequest{Name: "bob"})
	if _, err := h2.admin().GetAgent(c, "reviewer"); err == nil {
		t.Fatal("alias reviewer still routes to the new bob")
	}
}

// Admin routes answer 401 for a missing or unknown token and 403 for a valid
// agent token, and the CLI treats both as a permission error (exit 4).
func TestAdminRoutesDistinguish401And403(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	agentTok := h.register(api.RegisterRequest{Name: "alice"})
	for _, tc := range []struct {
		token, code string
	}{{"", "unauthenticated"}, {"wrong-admin-token", "unauthenticated"}, {agentTok, "permission_denied"}} {
		_, err := h.client(tc.token).Register(c, api.RegisterRequest{Name: "x"}, false)
		if e, ok := client.AsError(err); !ok || e.Code != tc.code {
			t.Errorf("token %q: %v, want %s", tc.token[:min(len(tc.token), 6)], err, tc.code)
		}
	}
	e := newCLI(t, h, filepath.Join(home, "x.yaml"))
	e.env["MYCO_ADMIN_TOKEN"] = "wrong-admin-token"
	if code, _, _ := e.run(c, "agent", "register", "--name", "x"); code != cli.ExitPermission {
		t.Fatalf("CLI with a wrong admin token: exit %d, want %d", code, cli.ExitPermission)
	}
}

// fakePi is a stand-in for the `pi` agent CLI (the "pi" preset): it prints
// the JSONL events myco parses, keeps one session id, and on resume answers
// with that id so the test can tell the same session was continued.
const fakePi = `#!/bin/sh
sess=pi-sess-1
case " $* " in
  *" --session "*) printf '{"type":"session","id":"%s"}\n' "$sess"
                   printf '{"type":"message_end","message":{"role":"assistant","content":"resumed %s in %s key=%s"}}\n' "$sess" "$PWD" "$FAKE_PROVIDER_KEY" ;;
  *)               printf '{"type":"session","id":"%s"}\n' "$sess"
                   printf '{"type":"message_end","message":{"role":"assistant","content":"ready key=%s"}}\n' "$FAKE_PROVIDER_KEY" ;;
esac
`

// agent spawn end to end: a machine joins with a link (and gets a machine
// node), its runner serves the node, another agent spawns a session in a
// directory there, and the new agent then answers by resuming that session.
func TestAgentSpawnEndToEnd(t *testing.T) {
	testutil.NeedsPOSIXShell(t) // fakePi
	home := isolatedHome(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "pi"), []byte(fakePi), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.MkdirAll(filepath.Join(home, "proj"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	link, err := h.admin().CreateEnrollment(c, api.CreateEnrollmentRequest{Kind: "link", AllowFrom: []string{"lead"}, MaxUses: 1, TTLSeconds: 3600}, false)
	if err != nil {
		t.Fatal(err)
	}
	lead := newCLI(t, h, filepath.Join(home, "lead.yaml"))
	coder := newCLI(t, h, filepath.Join(home, "coder.yaml"))
	lead.must(c, "agent", "register", "--name", "lead")
	coder.must(c, "agent", "register", "--name", "coder")

	// The machine joins; its machine node appears with the link's allow_from.
	box := newCLI(t, h, filepath.Join(home, ".myco", "config.yaml"))
	delete(box.env, "MYCO_HUB")
	delete(box.env, "MYCO_ADMIN_TOKEN")
	box.env["PATH"] = os.Getenv("PATH")
	out := box.must(c, "machine", "join", "--link", link.JoinURL, "--machine", "box", "--no-service")
	if !strings.Contains(out, `"machine_node": "box-machine"`) {
		t.Fatalf("machine join did not create the machine node: %s", out)
	}
	node, err := h.admin().GetAgent(c, "box-machine")
	if err != nil || node.Binding.Harness != "machine" || len(node.AllowFrom) != 1 || node.AllowFrom[0] != "lead" {
		t.Fatalf("machine node: %v %+v", err, node)
	}

	// Provider settings (like ANTHROPIC_BASE_URL / ANTHROPIC_API_KEY) are
	// captured once for the machine node; spawned sessions use them on the
	// first turn and inherit them afterwards. The runner itself never has
	// the variable, so only the env file can provide it.
	t.Setenv("FAKE_PROVIDER_KEY", "k1")
	box.must(c, "runner", "env", "capture", "--agent", "box-machine", "--var", "FAKE_PROVIDER_KEY")
	os.Unsetenv("FAKE_PROVIDER_KEY")

	// The machine's runner serves the node (and, later, the spawned agent).
	rctx, stop := context.WithCancel(c)
	done := make(chan struct{})
	go func() { defer close(done); box.run(rctx, "runner", "start", "--all", "--log-level", "error") }()
	t.Cleanup(func() { stop(); <-done })

	out = lead.must(c, "agent", "spawn", "--on", "box", "--dir", filepath.Join(home, "proj"), "--cli", "pi",
		"--name", "rev", "--description", "reviews proj", "--timeout", "20s")
	var sp struct{ Result api.SpawnResult }
	if err := json.Unmarshal([]byte(out), &sp); err != nil {
		t.Fatalf("spawn output: %s", out)
	}
	r := sp.Result
	if r.Agent != "box-rev" || r.SessionID != "pi-sess-1" || r.CLI != "pi" || r.Access != "workspace" || r.Reply != "ready key=k1" ||
		r.Workdir != filepath.Join(home, "proj") {
		t.Fatalf("spawn result: %+v", r)
	}
	a, err := h.admin().GetAgent(c, "box-rev")
	if err != nil || a.Binding.SessionID != "pi-sess-1" || a.Binding.Workdir != filepath.Join(home, "proj") || a.Description != "reviews proj" {
		t.Fatalf("spawned agent: %v %+v", err, a)
	}

	// The new agent is served right away and resumes the spawned session.
	out = lead.must(c, "message", "send", "--to", "box-rev", "--body", "status?", "--wait", "20s")
	if !strings.Contains(out, "resumed pi-sess-1 in "+filepath.Join(home, "proj")+" key=k1") {
		t.Fatalf("the spawned agent did not resume its session with the machine's provider settings: %s", out)
	}

	// The machine's rules: senders outside allow_from, and directories
	// outside home, are refused.
	if code, _, _ := coder.run(c, "agent", "spawn", "--on", "box", "--dir", filepath.Join(home, "proj"), "--cli", "pi", "--timeout", "5s"); code != cli.ExitPermission {
		t.Fatalf("coder spawn: exit %d, want %d", code, cli.ExitPermission)
	}
	code, _, errOut := lead.run(c, "agent", "spawn", "--on", "box", "--dir", t.TempDir(), "--cli", "pi", "--timeout", "20s")
	if code != cli.ExitPermission || !strings.Contains(errOut, "outside the allowed directories") {
		t.Fatalf("spawn outside home: exit %d %s", code, errOut)
	}

	// The admin (console) starts sessions on the machine too: its runner
	// reported the machine, the machine node makes it able to, and the
	// node's allow_from (only lead) does not apply; its directory rules do.
	var boxID string
	for i := 0; i < 50 && boxID == ""; i++ {
		ms, err := h.admin().Machines(c)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			if m.CanSpawn && !m.Legacy && m.Name == "box" {
				boxID = m.ID
			}
		}
		if boxID == "" {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if boxID == "" {
		t.Fatal("the box machine never reported itself with a usable machine node")
	}
	res, err := h.admin().SpawnOnMachine(c, boxID, api.SpawnRequest{CLI: "pi", Dir: filepath.Join(home, "proj"), Name: "rev2"}, 20*time.Second)
	if err != nil || !strings.HasPrefix(res.Agent, "box-") || res.Reply != "ready key=k1" {
		t.Fatalf("admin spawn: %v %+v", err, res)
	}
	_, err = h.admin().SpawnOnMachine(c, boxID, api.SpawnRequest{CLI: "pi", Dir: t.TempDir()}, 20*time.Second)
	if e, ok := client.AsError(err); !ok || e.Code != "permission_denied" || !strings.Contains(e.Message, "outside the allowed") {
		t.Fatalf("admin spawn outside the machine's directories: %v", err)
	}
	if _, err := h.client("").SpawnOnMachine(c, boxID, api.SpawnRequest{CLI: "pi", Dir: "~"}, time.Second); err == nil {
		t.Fatal("spawn without a token")
	}

	// Only a machine's own credentials can create a machine node.
	plain, err := h.admin().CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "x-"}, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.client(plain.Token).Enroll(c, api.EnrollRequest{Binding: api.Binding{Harness: "machine", SessionID: "machine:h:u", Environment: "h"}}, false)
	if e, ok := client.AsError(err); !ok || e.Code != "permission_denied" {
		t.Fatalf("plain enrollment created a machine node: %v", err)
	}
}

// fakeReleases serves a release API (latest = tag) and the release assets for
// this platform, like GitHub does.
func fakeReleases(t *testing.T, tag string, bin []byte, sum string) *httptest.Server {
	t.Helper()
	asset := release.AssetName(runtime.GOOS, runtime.GOARCH)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/latest":
			fmt.Fprintf(w, `{"tag_name":%q}`, tag)
		case "/dl/" + tag + "/SHA256SUMS":
			fmt.Fprintf(w, "%s  %s\n", sum, asset)
		case "/dl/" + tag + "/" + asset:
			w.Write(bin)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setVersion(t *testing.T, v string) {
	old := cli.Version
	cli.Version = v
	t.Cleanup(func() { cli.Version = old })
}

// The hub reports its build and the newest release; version and doctor show
// what is out of date.
func TestHubReportsVersionsAndClientsWarn(t *testing.T) {
	home := isolatedHome(t)
	setVersion(t, "v0.16.6")
	c := ctx(t)
	rel := fakeReleases(t, "v0.16.7", nil, "")
	check := &release.Checker{API: rel.URL + "/api/latest", HTTP: rel.Client()}
	h, err := hubserver.Build(hubserver.Options{DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: adminToken,
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version: "v0.16.5", UpdateCheck: check})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler)
	t.Cleanup(func() { srv.Close(); h.Close() })
	x := &hub{t: t, url: srv.URL}

	health, err := x.client("").Health(c)
	if err != nil || health.ServerVersion != "v0.16.5" || health.LatestVersion != "" || health.UpdateAvailable {
		t.Fatalf("before the first check: %v %+v", err, health)
	}
	if err := check.Check(c); err != nil {
		t.Fatal(err)
	}
	health, _ = x.client("").Health(c)
	if health.LatestVersion != "v0.16.7" || !health.UpdateAvailable {
		t.Fatalf("after the check: %+v", health)
	}

	e := newCLI(t, x, filepath.Join(home, "a.yaml"))
	e.must(c, "agent", "register", "--name", "alice")
	out := e.must(c, "version")
	for _, want := range []string{`"version": "v0.16.6"`, `"version": "v0.16.5"`, `"latest": "v0.16.7"`, `"update_available": true`} {
		if !strings.Contains(out, want) {
			t.Errorf("version output lacks %s:\n%s", want, out)
		}
	}
	_, out, _ = e.run(c, "doctor")
	for _, want := range []string{"the hub runs v0.16.5, older than this machine's v0.16.6", "myco v0.16.7 is available"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
}

// myco upgrade: verified download, atomic replace, nothing changed on a bad
// checksum, and dev builds are not replaced without an explicit version.
func TestUpgradeReplacesBinaryAfterVerifying(t *testing.T) {
	home := isolatedHome(t)
	setVersion(t, "v0.16.6")
	c := ctx(t)
	newBin := []byte("#!/bin/sh\necho v0.16.7\n")
	sum := sha256.Sum256(newBin)
	rel := fakeReleases(t, "v0.16.7", newBin, hex.EncodeToString(sum[:]))
	target := filepath.Join(t.TempDir(), "myco")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &cliEnv{t: t, env: map[string]string{"HOME": home, "MYCO_FORMAT": "json", "PATH": os.Getenv("PATH"),
		release.EnvAPI: rel.URL + "/api/latest", release.EnvDownload: rel.URL + "/dl", cli.EnvUpgradeTarget: target}}

	out := e.must(c, "upgrade", "--check")
	if !strings.Contains(out, `"update_available": true`) || !strings.Contains(out, `"target": "v0.16.7"`) {
		t.Fatalf("check: %s", out)
	}
	if b, _ := os.ReadFile(target); string(b) != "old binary" {
		t.Fatal("--check must not change anything")
	}
	out = e.must(c, "upgrade", "--no-restart")
	if b, _ := os.ReadFile(target); string(b) != string(newBin) {
		t.Fatalf("binary not replaced: %s", out)
	}
	if st, _ := os.Stat(target); osutil.UnixPermissions && st.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", st.Mode())
	}

	// A tampered download is refused and the binary stays as it was.
	os.WriteFile(target, []byte("old binary"), 0o755)
	tampered := fakeReleases(t, "v0.16.7", newBin, strings.Repeat("a", 64))
	e.env[release.EnvAPI], e.env[release.EnvDownload] = tampered.URL+"/api/latest", tampered.URL+"/dl"
	code, _, errOut := e.run(c, "upgrade", "--no-restart")
	if code == 0 || !strings.Contains(errOut, "checksum mismatch") {
		t.Fatalf("tampered upgrade: exit %d %s", code, errOut)
	}
	if b, _ := os.ReadFile(target); string(b) != "old binary" {
		t.Fatal("a failed upgrade changed the binary")
	}

	// Up to date: nothing to do. Dev builds need an explicit version.
	setVersion(t, "v0.16.7")
	if out := e.must(c, "upgrade", "--no-restart"); !strings.Contains(out, `"update_available": false`) {
		t.Fatalf("up to date: %s", out)
	}
	setVersion(t, "dev")
	if code, _, _ := e.run(c, "upgrade", "--no-restart"); code != cli.ExitUsage {
		t.Fatalf("dev build: exit %d", code)
	}
}

// Ordinary commands never wait for the version check: with a hub that
// accepts connections but never answers health checks, a command that does
// not need the hub still returns at once; notices come from the cache, at
// most once a day, and only on a terminal.
func TestUpdateNoticesNeverBlock(t *testing.T) {
	home := isolatedHome(t)
	setVersion(t, "v0.16.6")
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never answers
	}))
	t.Cleanup(hang.Close)
	cfg := filepath.Join(home, ".myco", "config.yaml")
	os.MkdirAll(filepath.Dir(cfg), 0o700)
	os.WriteFile(cfg, []byte("hub: "+hang.URL+"\n"), 0o600)

	run := func(tty bool) (time.Duration, string) {
		var out, errb bytes.Buffer
		start := time.Now()
		cli.Run(context.Background(), []string{"schema", "--all"}, strings.NewReader(""), &out, &errb,
			func(k string) string { return map[string]string{"HOME": home, "PATH": os.Getenv("PATH")}[k] }, tty)
		return time.Since(start), errb.String()
	}
	if d, _ := run(true); d > 2*time.Second {
		t.Fatalf("a command waited %s for the update check", d)
	}

	// A cached newer release is shown once, then not again for a day.
	cache := filepath.Join(home, ".myco", "update-check.json")
	os.WriteFile(cache, []byte(`{"hub":"`+hang.URL+`","hub_version":"v0.16.6","hub_known":true,"latest":"v0.16.9","checked_at":"`+
		time.Now().Format(time.RFC3339)+`"}`), 0o600)
	if _, errOut := run(false); strings.Contains(errOut, "notice") {
		t.Fatal("notice printed without a terminal")
	}
	if _, errOut := run(true); !strings.Contains(errOut, "myco v0.16.9 is available") {
		t.Fatalf("no notice: %q", errOut)
	}
	if _, errOut := run(true); strings.Contains(errOut, "notice") {
		t.Fatal("notice repeated within a day")
	}
	// MYCO_NO_UPDATE_CHECK=1 silences everything.
	os.WriteFile(cache, []byte(`{"latest":"v0.16.9"}`), 0o600)
	var errb bytes.Buffer
	cli.Run(context.Background(), []string{"schema", "--all"}, strings.NewReader(""), io.Discard, &errb,
		func(k string) string {
			return map[string]string{"HOME": home, release.EnvNoCheck: "1"}[k]
		}, true)
	if strings.Contains(errb.String(), "notice") {
		t.Fatal("notice despite MYCO_NO_UPDATE_CHECK=1")
	}
}

// The runner reads the agent's env file before every turn: adding
// runner.env_file to the profile (or re-capturing it) applies to the next
// message without restarting the runner.
func TestRunnerPicksUpEnvFileWithoutRestart(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	alice := newCLI(t, h, filepath.Join(home, "alice.yaml"))
	workerCfg := filepath.Join(home, "worker.yaml")
	worker := newCLI(t, h, workerCfg)
	alice.must(c, "agent", "register", "--name", "alice")
	worker.must(c, "agent", "register", "--name", "worker", "--harness", "shell")

	rctx, stop := context.WithCancel(c)
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.run(rctx, "runner", "start", "--harness", "shell", "--command", `echo "key=$PROVIDER_KEY"`, "--workdir", t.TempDir(), "--log-level", "error")
	}()
	t.Cleanup(func() { stop(); <-done })

	ask := func() string {
		return alice.must(c, "message", "send", "--to", "worker", "--body", "key?", "--wait", "20s")
	}
	if out := ask(); !strings.Contains(out, "key=") || strings.Contains(out, "key=k1") {
		t.Fatalf("before the env file: %s", out)
	}
	envFile := filepath.Join(home, "worker.env")
	if err := os.WriteFile(envFile, []byte("PROVIDER_KEY=k1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(workerCfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Runner.EnvFile = envFile
	if err := config.Save(workerCfg, cfg); err != nil {
		t.Fatal(err)
	}
	if out := ask(); !strings.Contains(out, "key=k1") {
		t.Fatalf("the running runner did not pick up the new env file: %s", out)
	}
	if err := os.WriteFile(envFile, []byte("PROVIDER_KEY=k2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := ask(); !strings.Contains(out, "key=k2") {
		t.Fatalf("a re-captured env file did not apply: %s", out)
	}
}

// Console tokens: per-device admin credentials. Only the hub's admin token
// manages them; a console token can do admin work (list, kick, overview)
// but not mint or revoke console tokens; a revoked one stops at once.
func TestConsoleTokens(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	aliceTok := h.register(api.RegisterRequest{Name: "alice"})

	// The CLI on the hub machine (admin token) issues one; the plain token is
	// printed once and is not stored.
	adminCLI := newCLI(t, h, filepath.Join(home, "admin.yaml"))
	out := adminCLI.must(c, "console", "token", "create", "--name", "glenn-mac")
	var created struct {
		ConsoleToken api.ConsoleToken `json:"console_token"`
		Token        string           `json:"token"`
		Hub          string           `json:"hub"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("console token create: %s", out)
	}
	if !strings.HasPrefix(created.Token, app.ConsoleTokenPrefix) || created.ConsoleToken.Name != "glenn-mac" || !created.ConsoleToken.Active || created.Hub == "" {
		t.Fatalf("created: %+v", created)
	}
	list := adminCLI.must(c, "console", "token", "list")
	if strings.Contains(list, created.Token) || !strings.Contains(list, created.ConsoleToken.ID) {
		t.Fatalf("list must show the token's id and never the token: %s", list)
	}

	// It works as an admin credential ...
	con := h.client(created.Token)
	if _, err := con.ListEnrollments(c); err != nil {
		t.Fatalf("console token on an admin route: %v", err)
	}
	if _, err := con.Register(c, api.RegisterRequest{Name: "bob"}, false); err != nil {
		t.Fatalf("console token registering an agent: %v", err)
	}
	// ... and records when it was last used ...
	toks, err := h.admin().ListConsoleTokens(c)
	if err != nil || len(toks) != 1 || toks[0].LastUsedAt == nil {
		t.Fatalf("last used not recorded: %v %+v", err, toks)
	}
	// ... but cannot manage console tokens (403), and neither can agents.
	for name, cl := range map[string]*client.Client{"console": con, "agent": h.client(aliceTok)} {
		_, err := cl.CreateConsoleToken(c, "evil")
		if e, ok := client.AsError(err); !ok || e.Code != "permission_denied" {
			t.Errorf("%s token creating a console token: %v, want permission_denied", name, err)
		}
		_, err = cl.RevokeConsoleToken(c, created.ConsoleToken.ID)
		if e, ok := client.AsError(err); !ok || e.Code != "permission_denied" {
			t.Errorf("%s token revoking a console token: %v, want permission_denied", name, err)
		}
	}
	// A token that only looks like one is unauthenticated.
	_, err = h.client(app.ConsoleTokenPrefix + "made-up").ListEnrollments(c)
	if e, ok := client.AsError(err); !ok || e.Code != "unauthenticated" {
		t.Fatalf("made-up console token: %v", err)
	}

	// Revoked: unauthenticated right away; revoking again is fine.
	adminCLI.must(c, "console", "token", "revoke", "--id", created.ConsoleToken.ID)
	adminCLI.must(c, "console", "token", "revoke", "--id", created.ConsoleToken.ID)
	_, err = con.ListEnrollments(c)
	if e, ok := client.AsError(err); !ok || e.Code != "unauthenticated" {
		t.Fatalf("revoked console token: %v, want unauthenticated", err)
	}
}

// Kick signs an agent out: its token stops working at once, it shows as
// offline, its record and queued messages stay, and it can come back.
func TestKickSignsAnAgentOut(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	leadTok := h.register(api.RegisterRequest{Name: "lead"})
	workerTok := h.register(api.RegisterRequest{Name: "worker", Labels: []string{"group=a"}})
	if _, err := h.client(workerTok).Me(c); err != nil {
		t.Fatal(err)
	}
	if _, err := h.client(leadTok).Send(c, api.SendRequest{To: "worker", Body: "queued before the kick"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.client(leadTok).KickAgent(c, "worker", false); err == nil {
		t.Fatal("an agent token kicked another agent")
	}

	adminCLI := newCLI(t, h, filepath.Join(home, "admin.yaml"))
	if code, _, _ := adminCLI.run(c, "agent", "kick", "--name", "worker", "--dry-run"); code != cli.ExitDryRun {
		t.Fatalf("dry run: exit %d", code)
	}
	if _, err := h.client(workerTok).Me(c); err != nil {
		t.Fatalf("a dry run must not sign the agent out: %v", err)
	}
	adminCLI.must(c, "agent", "kick", "--name", "worker")

	_, err := h.client(workerTok).Me(c)
	if e, ok := client.AsError(err); !ok || e.Code != "unauthenticated" {
		t.Fatalf("kicked agent's token: %v, want unauthenticated", err)
	}
	a, err := h.admin().GetAgent(c, "worker")
	if err != nil || a.Online || len(a.Labels) != 1 {
		t.Fatalf("kicked agent must stay, offline, with its labels: %v %+v", err, a)
	}
	// It comes back with a new token, and its queued message is still there.
	back, err := h.admin().Register(c, api.RegisterRequest{Name: "worker", RotateToken: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := h.client(back.Token).Pull(c, 10, 0)
	if err != nil || len(msgs) != 1 || msgs[0].Body != "queued before the kick" {
		t.Fatalf("queued message after coming back: %v %+v", err, msgs)
	}
	if _, err := h.admin().KickAgent(c, "nobody", false); err == nil {
		t.Fatal("kicking an unknown agent must fail")
	}
}

// Machines are identified by the fingerprint their runners report: the same
// device is one machine however many agents, joins or host names it has;
// agents of runners that have not reported yet are grouped by host name;
// people are not on machines; machine nodes are not listed as a machine's
// agents but make it able to start sessions.
func TestMachinesAreUniqueAndSeparateFromNodes(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	tok := map[string]string{}
	for _, r := range []api.RegisterRequest{
		{Name: "mac-claude", Binding: api.Binding{Harness: "claude", Environment: "Glenns-MacBook.local"}},
		{Name: "mac-codex", Binding: api.Binding{Harness: "codex", Environment: "glenns-macbook"}},
		{Name: "old-codex", Binding: api.Binding{Harness: "codex", Environment: "192.168.1.210"}},
		{Name: "cloud-a", Binding: api.Binding{Harness: "claude-cloud", Environment: "claude-cloud_default"}},
		{Name: "vps-shell", Binding: api.Binding{Harness: "exec", Environment: "srv1"}},
		{Name: "glenn"},
	} {
		tok[r.Name] = h.register(r)
	}
	// Before any report: grouped by host name (case and ".local" ignored).
	ms, err := h.admin().Machines(c)
	if err != nil {
		t.Fatal(err)
	}
	byName := func(ms []api.Machine) map[string]api.Machine {
		out := map[string]api.Machine{}
		for _, m := range ms {
			out[m.Name] = m
		}
		return out
	}
	m := byName(ms)
	if len(ms) != 3 || len(m["Glenns-MacBook.local"].Agents) != 2 || !m["Glenns-MacBook.local"].Legacy || len(m["srv1"].Agents) != 1 {
		t.Fatalf("legacy grouping: %+v", ms)
	}
	for _, x := range ms {
		for _, a := range x.Agents {
			if a == "glenn" {
				t.Fatal("a person is not on a machine")
			}
		}
	}

	// Both Mac agents report the same fingerprint (new runners), one even
	// after a host name change: one machine, named after its join name.
	fp := strings.Repeat("ab", 16)
	r1 := api.MachineReport{Fingerprint: fp, Name: "glenn-mac", Hostname: "Glenns-MacBook.local", OS: "darwin", Arch: "arm64", MycoVersion: "v0.16.9"}
	if err := h.client(tok["mac-claude"]).ReportMachine(c, r1); err != nil {
		t.Fatal(err)
	}
	r2 := r1
	r2.Hostname, r2.Name = "Glenns-MacBook-2.local", "something-else"
	if err := h.client(tok["mac-codex"]).ReportMachine(c, r2); err != nil {
		t.Fatal(err)
	}
	if err := h.client(tok["mac-claude"]).ReportRuntime(c, api.Runtime{CLI: "claude", CLIVersion: "2.1.240", Model: "claude-opus-5-5", ModelSource: "configured", Access: "workspace"}); err != nil {
		t.Fatal(err)
	}
	ms, _ = h.admin().Machines(c)
	m = byName(ms)
	mac, ok := m["glenn-mac"]
	if !ok || mac.Legacy || len(mac.Agents) != 2 || mac.OS != "darwin" || mac.Hostname != "Glenns-MacBook-2.local" || len(ms) != 3 {
		t.Fatalf("one machine for one device: %+v", ms)
	}
	if err := h.client(tok["old-codex"]).ReportMachine(c, api.MachineReport{Fingerprint: "x"}); err == nil {
		t.Fatal("a too-short fingerprint must be refused")
	}
	if err := h.client(tok["cloud-a"]).ReportMachine(c, api.MachineReport{Fingerprint: strings.Repeat("cd", 16)}); err == nil {
		t.Fatal("a cloud session must not create a machine")
	}

	// The overview (admin) carries machines, kinds and runtimes.
	ov, err := h.admin().Overview(c, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Machines) != 3 {
		t.Fatalf("overview machines: %+v", ov.Machines)
	}
	kinds := map[string]string{}
	for _, a := range ov.Agents {
		kinds[a.Agent.Name] = a.Agent.Kind
		if a.Agent.Name == "mac-claude" && (a.Agent.MachineID != mac.ID || a.Agent.Runtime == nil || a.Agent.Runtime.Model != "claude-opus-5-5") {
			t.Fatalf("mac-claude in the overview: %+v", a.Agent)
		}
		if a.Agent.Name == "old-codex" && !strings.HasPrefix(a.Agent.MachineID, "env:") {
			t.Fatalf("a legacy agent still gets its (legacy) machine: %+v", a.Agent)
		}
		if (a.Agent.Name == "cloud-a" || a.Agent.Name == "glenn") && a.Agent.MachineID != "" {
			t.Fatalf("cloud sessions and people are not on machines: %+v", a.Agent)
		}
	}
	if kinds["vps-shell"] != "shell" || kinds["glenn"] != "human" || kinds["cloud-a"] != "agent" {
		t.Fatalf("kinds: %v", kinds)
	}
	// An agent's own overview does not list machines.
	if ov, _ := h.client(tok["mac-claude"]).Overview(c, 5); len(ov.Machines) != 0 {
		t.Fatal("machines are for the admin view")
	}

	// Renaming: real machines yes, legacy ones say what to do.
	if err := h.admin().RenameMachine(c, mac.ID, "Glenn 的 Mac"); err != nil {
		t.Fatal(err)
	}
	err = h.admin().RenameMachine(c, m["srv1"].ID, "VPS")
	if e, ok := client.AsError(err); !ok || e.Code != "invalid_state" {
		t.Fatalf("renaming a legacy machine: %v", err)
	}
	if err := h.client(tok["mac-claude"]).RenameMachine(c, mac.ID, "x"); err == nil {
		t.Fatal("an agent renamed a machine")
	}
	// No machine node: sessions cannot be started there.
	_, err = h.admin().SpawnOnMachine(c, mac.ID, api.SpawnRequest{CLI: "claude", Dir: "~"}, 5*time.Second)
	if e, ok := client.AsError(err); !ok || e.Code != "invalid_state" || !strings.Contains(e.Suggestion, "runner") {
		t.Fatalf("spawn without a machine node: %v", err)
	}
}

// `myco console link` prints one link with the hub's address and a fresh
// console token; opened in a browser the hub explains how to use it.
func TestConsoleAppLink(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	adminCLI := newCLI(t, h, filepath.Join(home, "admin.yaml"))
	out := adminCLI.must(c, "console", "link", "--name", "work-mac")
	var res struct{ Link, Hub string }
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("console link: %s", out)
	}
	base, tok, ok := strings.Cut(res.Link, "/c/")
	if !ok || base != h.url || !strings.HasPrefix(tok, app.PairingCodePrefix) {
		t.Fatalf("link %q", res.Link)
	}
	// The link carries a one-time code: the app trades it for its own token.
	paired, err := h.client("").Pair(c, tok, "work-mac")
	if err != nil {
		t.Fatalf("pairing: %v", err)
	}
	if _, err := h.client(paired.Token).ListEnrollments(c); err != nil {
		t.Fatalf("the paired token must work: %v", err)
	}
	if _, err := h.client("").Pair(c, tok, "again"); err == nil {
		t.Fatal("the link must work only once")
	}
	for _, path := range []string{"/c/" + tok, "/c/" + app.ConsoleTokenPrefix + "made-up"} {
		req, _ := http.NewRequest("GET", h.url+path, nil)
		req.Header.Set("Accept", "text/html")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), "myco://add-hub?link=") || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, body)
		}
	}
}

// The administrator talks to agents from the console app: as
// "@console:<device>", past allow_from; conversations list the threads,
// with search and an agent filter; tasks can be corrected (steer).
func TestConsoleConversations(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	workerTok := h.register(api.RegisterRequest{Name: "worker", AllowFrom: []string{"lead"}})
	otherTok := h.register(api.RegisterRequest{Name: "other"})
	h.register(api.RegisterRequest{Name: "lead"})
	ct, err := h.admin().CreateConsoleToken(c, "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	con := h.client(ct.Token)
	worker, other := h.client(workerTok), h.client(otherTok)

	sent, err := con.ConsoleSend(c, api.SendRequest{To: "worker", Body: "请把 README 的安装一节改短"})
	if err != nil || sent.Message.From != "@console:owner" {
		t.Fatalf("console send (worker only allows lead): %v %+v", err, sent)
	}
	if _, err := con.ConsoleSend(c, api.SendRequest{To: "other", Body: "检查一下 CI"}); err != nil {
		t.Fatal(err)
	}
	msgs, err := worker.Pull(c, 10, 0)
	if err != nil || len(msgs) != 1 || msgs[0].From != "@console:owner" {
		t.Fatalf("worker pull: %v %+v", err, msgs)
	}
	if _, err := con.ConsoleSteer(c, sent.Message.ID, "只改安装一节"); err != nil {
		t.Fatalf("steer: %v", err)
	}
	if _, err := worker.Reply(c, sent.Message.ID, "改好了：```bash\nmyco join\n```", false); err != nil {
		t.Fatal(err)
	}

	cs, err := con.Conversations(c, "", "", 0, 0)
	if err != nil || len(cs) != 2 || cs[0].Agent != "worker" || cs[0].Messages != 3 || cs[0].Last.Type != "reply" || cs[0].First.Body != "请把 README 的安装一节改短" {
		t.Fatalf("conversations: %v %+v", err, cs)
	}
	if cs, _ := con.Conversations(c, "other", "", 0, 0); len(cs) != 1 || cs[0].Agent != "other" {
		t.Fatalf("agent filter: %+v", cs)
	}
	if cs, _ := con.Conversations(c, "", "myco join", 0, 0); len(cs) != 1 || cs[0].Agent != "worker" {
		t.Fatalf("search: %+v", cs)
	}
	if cs, _ := con.Conversations(c, "", "100%", 0, 0); len(cs) != 0 {
		t.Fatalf("LIKE wildcards must be literal: %+v", cs)
	}
	// Same conversation, next message: the same thread.
	next, err := con.ConsoleSend(c, api.SendRequest{To: "worker", Body: "再加一个例子", ThreadID: sent.Message.ThreadID})
	if err != nil || next.Message.ThreadID != sent.Message.ThreadID {
		t.Fatalf("follow-up: %v %+v", err, next)
	}

	// Agents cannot use the console routes, nor write to the console.
	if _, err := other.ConsoleSend(c, api.SendRequest{To: "worker", Body: "x"}); err == nil {
		t.Fatal("an agent sent as the console")
	}
	if _, err := other.Send(c, api.SendRequest{To: "@console:owner", Body: "x"}, false); err == nil {
		t.Fatal("an agent messaged the console directly")
	}
	lead := h.client(h.register(api.RegisterRequest{Name: "lead2"}))
	task, _ := lead.Send(c, api.SendRequest{To: "other", Body: "y"}, false)
	if _, err := con.ConsoleSteer(c, task.Message.ID, "z"); err == nil {
		t.Fatal("the console corrected an agent's task")
	}
	if _, err := con.ConsoleStop(c, task.Message.ID); err == nil {
		t.Fatal("the console stopped an agent's task")
	}

	// Stop: the task is closed as failed, once.
	run, _ := con.ConsoleSend(c, api.SendRequest{To: "worker", Body: "跑很久的任务"})
	if m, err := con.ConsoleStop(c, run.Message.ID); err != nil || m.Status != "failed" || m.Error != "stopped by the sender" {
		t.Fatalf("stop: %v %+v", err, m)
	}
	if _, err := con.ConsoleStop(c, run.Message.ID); err == nil {
		t.Fatal("stopped a finished task")
	}
}
