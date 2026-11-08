package machinenode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

type fakeCLI struct {
	got  harness.Request
	opts harness.Options
	out  harness.Result
	err  error
}

func (f *fakeCLI) Name() string { return "fake" }
func (f *fakeCLI) Run(_ context.Context, r harness.Request) (harness.Result, error) {
	f.got = r
	return f.out, f.err
}

type rig struct {
	home     string
	node     *Node
	cli      *fakeCLI
	enrolled []Plan
	spawned  int
}

func newRig(t *testing.T) *rig {
	t.Helper()
	home := t.TempDir()
	for _, d := range []string{"code/app", ".ssh/keys", ".myco"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	r := &rig{home: home, cli: &fakeCLI{out: harness.Result{Output: " ready ", SessionID: "sess-1"}}}
	r.node = &Node{Machine: "wsl", Prefix: "wsl-", Policy: Policy{Home: home}, Senders: []string{"lead"},
		Deps: Deps{
			NewHarness: func(cli string, opts harness.Options) (harness.Harness, error) {
				r.cli.opts = opts
				return r.cli, nil
			},
			Installed: func(string) bool { return true },
			Enroll: func(_ context.Context, p Plan, _ api.SpawnRequest, sid string) (string, error) {
				if sid != "sess-1" {
					t.Errorf("enrolled session %q", sid)
				}
				r.enrolled = append(r.enrolled, p)
				return p.Name, nil
			},
			Spawned: func() { r.spawned++ },
		}}
	return r
}

func (r *rig) run(t *testing.T, from string, req api.SpawnRequest) api.SpawnResult {
	t.Helper()
	b, _ := json.Marshal(req)
	out, err := r.node.Run(context.Background(), harness.Request{From: from, Body: string(b), Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	var res api.SpawnResult
	if err := json.Unmarshal([]byte(out.Output), &res); err != nil {
		t.Fatalf("result is not a SpawnResult: %q", out.Output)
	}
	return res
}

func TestSpawnHappyPath(t *testing.T) {
	r := newRig(t)
	res := r.run(t, "lead", api.SpawnRequest{CLI: "claude", Dir: "~/code/app", Name: "reviewer", Description: "reviews the app", Prompt: "read README"})
	if res.Denied || res.Error != "" {
		t.Fatalf("spawn failed: %+v", res)
	}
	// The resolved path (on Windows the temp dir is an 8.3 short name).
	want, _ := filepath.EvalSymlinks(filepath.Join(r.home, "code", "app"))
	if res.Agent != "wsl-reviewer" || res.SessionID != "sess-1" || res.Workdir != want || res.Access != "workspace" || res.Reply != "ready" {
		t.Fatalf("result: %+v", res)
	}
	if r.cli.got.Workdir != want || r.cli.opts.Access != harness.AccessWorkspace {
		t.Fatalf("first turn ran with %+v / %+v", r.cli.got, r.cli.opts)
	}
	for _, s := range []string{`"lead"`, "wsl-reviewer", "reviews the app", "read README", "Do not start any work yet"} {
		if !strings.Contains(r.cli.got.Prompt, s) {
			t.Errorf("first prompt lacks %q:\n%s", s, r.cli.got.Prompt)
		}
	}
	if len(r.enrolled) != 1 || r.spawned != 1 {
		t.Fatalf("enrolled %d, spawned signal %d", len(r.enrolled), r.spawned)
	}
}

func TestDefaultNameAndCLI(t *testing.T) {
	r := newRig(t)
	res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"})
	if res.Agent != "wsl-app-claude" || res.CLI != "claude" {
		t.Fatalf("defaults: %+v", res)
	}
	if got := r.node.name("wsl-already-prefixed", "codex", "/x"); got != "wsl-already-prefixed" {
		t.Errorf("prefix added twice: %s", got)
	}
	if got := slug("Mon Projet_été!"); got != "mon-projet-t" {
		t.Errorf("slug: %q", got)
	}
}

func TestSendersFromAllowFrom(t *testing.T) {
	r := newRig(t)
	if res := r.run(t, "stranger", api.SpawnRequest{Dir: "~/code/app"}); !res.Denied || !strings.Contains(res.Error, "allow_from") {
		t.Fatalf("stranger: %+v", res)
	}
	// The live allow_from wins over the start-up copy (admin changed it).
	r.node.CurrentSenders = func() ([]string, error) { return []string{"stranger"}, nil }
	if res := r.run(t, "stranger", api.SpawnRequest{Dir: "~/code/app"}); res.Denied {
		t.Fatalf("stranger after allow_from change: %+v", res)
	}
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"}); !res.Denied {
		t.Fatal("lead was removed from allow_from but still allowed")
	}
	// Inherited from a link without allow_from: any agent (rule agreed for spawn).
	r.node.CurrentSenders = func() ([]string, error) { return []string{}, nil }
	if res := r.run(t, "anyone", api.SpawnRequest{Dir: "~/code/app"}); res.Denied {
		t.Fatalf("empty allow_from should admit any agent: %+v", res)
	}
	if len(r.enrolled) != 2 {
		t.Fatalf("enrolled %d", len(r.enrolled))
	}
}

func TestDirectoryPolicy(t *testing.T) {
	r := newRig(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(r.home, "code", "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(r.home, ".ssh"), filepath.Join(r.home, "code", "keys")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ dir, want string }{
		{"", "pass the directory"},
		{"code/app", "absolute"},
		{"~/missing", "does not exist"},
		{outside, "outside the allowed"},
		{"~/code/escape", "outside the allowed"}, // symlink out of home
		{"~/.ssh/keys", "credentials directory"}, // inside home but a credentials dir
		{"~/code/keys", "credentials directory"}, // symlink into one
		{"~/.myco", "credentials directory"},
	} {
		res := r.run(t, "lead", api.SpawnRequest{Dir: tc.dir})
		if !res.Denied || !strings.Contains(res.Error, tc.want) {
			t.Errorf("dir %q: %+v, want denied with %q", tc.dir, res, tc.want)
		}
	}
	// Narrowed roots in machine.yaml.
	r.node.Policy.Dirs = []string{"~/code/app"}
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"}); res.Denied {
		t.Fatalf("allowed root: %+v", res)
	}
	if err := os.MkdirAll(filepath.Join(r.home, "other"), 0o700); err != nil {
		t.Fatal(err)
	}
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/other"}); !res.Denied {
		t.Fatal("dir outside the narrowed roots was allowed")
	}
	if len(r.enrolled) != 1 {
		t.Fatalf("only the allowed request may enroll, got %d", len(r.enrolled))
	}
}

func TestAccessCap(t *testing.T) {
	r := newRig(t)
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app", Access: "full"}); !res.Denied || !strings.Contains(res.Error, "above this machine's limit (workspace)") {
		t.Fatalf("full under the default cap: %+v", res)
	}
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app", Access: "readonly"}); res.Denied || res.Access != "readonly" {
		t.Fatalf("readonly: %+v", res)
	}
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app", Access: "root"}); !res.Denied {
		t.Fatal("bogus access accepted")
	}
	r.node.Policy.MaxAccess = harness.AccessFull // set locally in machine.yaml
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app", Access: "full"}); res.Denied || res.Access != "full" {
		t.Fatalf("full with max_access full: %+v", res)
	}
	r.node.Policy.MaxAccess = harness.AccessReadonly
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"}); !res.Denied {
		t.Fatal("default workspace allowed under a readonly cap")
	}
}

func TestRefusalsAndFailuresAreResults(t *testing.T) {
	r := newRig(t)
	if res := r.run(t, "lead", api.SpawnRequest{CLI: "bash", Dir: "~/code/app"}); !res.Denied || !strings.Contains(res.Error, "supported CLIs") {
		t.Errorf("unsupported cli: %+v", res)
	}
	out, _ := r.node.Run(context.Background(), harness.Request{From: "lead", Body: "rm -rf /"})
	if !strings.Contains(out.Output, `"denied":true`) {
		t.Errorf("non-JSON body: %s", out.Output)
	}
	r.node.Deps.Installed = func(string) bool { return false }
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"}); !res.Denied || !strings.Contains(res.Error, "not installed") {
		t.Errorf("missing cli: %+v", res)
	}
	r.node.Deps.Installed = func(string) bool { return true }
	r.cli.err = errors.New("auth expired")
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"}); res.Denied || !strings.Contains(res.Error, "auth expired") {
		t.Errorf("first turn failure: %+v", res)
	}
	r.cli.err, r.cli.out = nil, harness.Result{Output: "ok"}
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"}); !strings.Contains(res.Error, "session id") {
		t.Errorf("no session id: %+v", res)
	}
	r.cli.out = harness.Result{Output: "ok", SessionID: "sess-1"}
	r.node.Deps.Enroll = func(context.Context, Plan, api.SpawnRequest, string) (string, error) {
		return "", errors.New("conflict: agent wsl-app-claude already exists")
	}
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"}); !strings.Contains(res.Error, "already exists") || !strings.Contains(res.Error, "sess-1") {
		t.Errorf("enroll failure: %+v", res)
	}
	if r.spawned != 0 {
		t.Fatalf("spawned signal on failure: %d", r.spawned)
	}
}

// Home itself may be a symlink (or a Windows short name): resolved working
// directories never start with it, so credential directories must still be
// refused by what they are, not by prefix.
func TestCredentialDirsWhenHomeIsASymlink(t *testing.T) {
	r := newRig(t)
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(r.home, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	r.node.Policy.Home = link
	for _, dir := range []string{"~/.ssh/keys", "~/.myco"} {
		if res := r.run(t, "lead", api.SpawnRequest{Dir: dir}); !res.Denied || !strings.Contains(res.Error, "credentials directory") {
			t.Errorf("dir %q with a symlinked home: %+v, want denied", dir, res)
		}
	}
	if res := r.run(t, "lead", api.SpawnRequest{Dir: "~/code/app"}); res.Denied {
		t.Errorf("an ordinary directory under a symlinked home: %+v", res)
	}
}
