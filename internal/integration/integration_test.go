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
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/cli"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/hubserver"
	"github.com/chenhg5/myco/internal/infra/sqlite"
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
	t.Setenv("HOME", home)
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
	if r := run("echo hello from ci"); r.code != 0 || !strings.Contains(r.out, "hello from ci") {
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
