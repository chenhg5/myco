package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/testutil"
)

func perm(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultPathsFromHome(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	def := DefaultPath()
	if want := filepath.Join(home, ".myco", "config.yaml"); def != want {
		t.Fatalf("DefaultPath = %s, want %s", def, want)
	}
	if got := Dir(def); got != filepath.Join(home, ".myco") {
		t.Fatalf("Dir = %s", got)
	}
	if got := ProfilesDir(def); got != filepath.Join(home, ".myco", "agents") {
		t.Fatalf("ProfilesDir = %s", got)
	}
	if got := ProfilePath(def, "vps-codex"); got != filepath.Join(home, ".myco", "agents", "vps-codex.yaml") {
		t.Fatalf("ProfilePath = %s", got)
	}
	if got := MachinePath(def); got != filepath.Join(home, ".myco", "machine.yaml") {
		t.Fatalf("MachinePath = %s", got)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope", "config.yaml"))
	if err != nil || c == nil || !reflect.DeepEqual(*c, Config{}) {
		t.Fatalf("Load(missing) = %+v, %v; want empty config", c, err)
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	writeFile(t, bad, "hub: [unterminated\n")
	if _, err := Load(bad); err == nil {
		t.Fatal("malformed yaml must be an error")
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("reading a directory must be an error, not an empty config")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	path := DefaultPath() // ~/.myco does not exist yet
	in := &Config{
		Hub: "https://hub.example", Agent: "mac-claude", Token: "myco_secret",
		Harness: "claude", SessionID: "sess-1",
		Runner: Runner{
			Harness: "template", Binary: "/usr/bin/x", Args: []string{"--a", "b c"},
			Command: "cat", Workdir: "/w", Timeout: "10m", Concurrency: 2, PollWait: "30s",
			EnvFile: "~/.myco/env", Model: "m", Access: "workspace",
			Template: &harness.TemplateSpec{Binary: "pi", Args: []string{"{prompt}"}, Output: "jsonl",
				Text: &harness.Extract{Path: "a.b", When: map[string]string{"type": "x"}, Append: true}},
			Exec: &harness.ExecSpec{Allow: []string{"git status"}, Timeout: "5s", MaxOutput: 10, Shell: "/bin/bash", InheritEnv: []string{"PATH"}},
		},
	}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
	// Overwrite with fewer fields: old values must not survive.
	if err := Save(path, &Config{Hub: "h2"}); err != nil {
		t.Fatal(err)
	}
	out, _ = Load(path)
	if !reflect.DeepEqual(*out, Config{Hub: "h2"}) {
		t.Fatalf("overwrite kept stale fields: %+v", out)
	}
	// omitempty: an empty config writes no keys.
	if err := Save(path, &Config{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); strings.TrimSpace(string(b)) != "{}" {
		t.Fatalf("empty config serialized as %q", b)
	}
}

func TestSavePermissions(t *testing.T) {
	testutil.NeedsUnixPermissions(t)
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if err := Save(path, &Config{Token: "myco_secret"}); err != nil {
		t.Fatal(err)
	}
	if p := perm(t, path); p != 0o600 {
		t.Errorf("config mode = %o, want 600 (holds a token)", p)
	}
	if p := perm(t, filepath.Dir(path)); p != 0o700 {
		t.Errorf("config dir mode = %o, want 700", p)
	}
	// A pre-existing world-readable file is replaced by a 600 one.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, &Config{Token: "myco_secret2"}); err != nil {
		t.Fatal(err)
	}
	if p := perm(t, path); p != 0o600 {
		t.Errorf("re-saved config mode = %o, want 600", p)
	}
}

// A stale, world-readable temp file from an earlier crash must not leak its
// mode into the saved file (Save used to reuse path+".tmp" via os.WriteFile).
func TestSaveIgnoresStaleTempFileMode(t *testing.T) {
	testutil.NeedsUnixPermissions(t)
	dir := t.TempDir()
	for _, name := range []string{"config.yaml", "machine.yaml"} {
		path := filepath.Join(dir, name)
		writeFile(t, path+".tmp", "")
		if err := os.Chmod(path+".tmp", 0o644); err != nil {
			t.Fatal(err)
		}
		var err error
		if name == "config.yaml" {
			err = Save(path, &Config{Token: "myco_secret"})
		} else {
			err = SaveMachine(filepath.Join(dir, "config.yaml"), &Machine{MachineHub: MachineHub{Hub: "https://hub", EnrollToken: "myco_enroll_secret"}})
		}
		if err != nil {
			t.Fatal(err)
		}
		if p := perm(t, path); p != 0o600 {
			t.Fatalf("%s mode = %o, want 600", name, p)
		}
	}
	// and the fresh temp files never linger
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestApplyEnvAndRedacted(t *testing.T) {
	env := map[string]string{EnvHub: "https://env-hub", EnvToken: "myco_env"}
	c := &Config{Hub: "https://file-hub", Token: "myco_file", Agent: "a"}
	c.ApplyEnv(func(k string) string { return env[k] })
	if c.Hub != "https://env-hub" || c.Token != "myco_env" || c.Agent != "a" {
		t.Fatalf("ApplyEnv: %+v", c)
	}
	c2 := &Config{Hub: "h", Token: "t"}
	c2.ApplyEnv(func(string) string { return "" })
	if c2.Hub != "h" || c2.Token != "t" {
		t.Fatalf("empty env must not override: %+v", c2)
	}
	r := c.Redacted()
	if r.Token != "***" || c.Token != "myco_env" || r.Hub != c.Hub {
		t.Fatalf("Redacted: r=%+v original=%+v", r, c)
	}
	if (Config{}).Redacted().Token != "" {
		t.Fatal("empty token must stay empty when redacted")
	}
}

func TestListProfilesAndFindBySession(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	def := DefaultPath()
	dir := ProfilesDir(def)

	ps, err := ListProfiles(dir)
	if err != nil || len(ps) != 0 {
		t.Fatalf("missing dir: %v %v", ps, err)
	}
	if _, ok := FindBySession(dir, "s1"); ok {
		t.Fatal("no profiles: nothing should match")
	}
	for name, sess := range map[string]string{"alpha": "s1", "beta": "s2"} {
		if err := Save(ProfilePath(def, name), &Config{Agent: name, SessionID: sess, Token: "t-" + name}); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, "notes.txt"), "ignored")
	writeFile(t, filepath.Join(dir, "gamma.yml"), "agent: gamma") // only *.yaml

	ps, err = ListProfiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].Config.Agent != "alpha" || ps[1].Config.Agent != "beta" ||
		ps[0].Path != ProfilePath(def, "alpha") {
		t.Fatalf("profiles = %+v", ps)
	}
	if p, ok := FindBySession(dir, "s2"); !ok || p.Config.Agent != "beta" || p.Config.Token != "t-beta" {
		t.Fatalf("FindBySession(s2) = %+v %v", p, ok)
	}
	if _, ok := FindBySession(dir, ""); ok {
		t.Fatal("empty session id must never match")
	}
	if _, ok := FindBySession(dir, "s3"); ok {
		t.Fatal("unknown session must not match")
	}

	// A malformed profile fails the listing, naming the file.
	writeFile(t, filepath.Join(dir, "broken.yaml"), "agent: [x\n")
	if _, err := ListProfiles(dir); err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Fatalf("broken profile: %v", err)
	}
	// FindBySession swallows the error and finds nothing.
	if _, ok := FindBySession(dir, "s1"); ok {
		t.Fatal("FindBySession with a broken profile: expected no match")
	}
}

func TestReadEnvFile(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	content := strings.Join([]string{
		"# comment",
		"",
		"A=1",
		"export B = two words ",
		`C="quoted value"`,
		`D='single'`,
		`E="unbalanced`,
		"F=",
		"G=a=b",
	}, "\n")
	writeFile(t, filepath.Join(home, "env"), content)
	got, err := ReadEnvFile("~/env")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A=1", "B=two words", "C=quoted value", "D=single", `E="unbalanced`, "F=", "G=a=b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadEnvFile = %q\nwant %q", got, want)
	}
	for _, bad := range []string{"NOEQUALS", "=v", "A B=1"} {
		p := filepath.Join(t.TempDir(), "env")
		writeFile(t, p, "OK=1\n"+bad+"\n")
		if _, err := ReadEnvFile(p); err == nil || !strings.Contains(err.Error(), ":2:") {
			t.Errorf("line %q: want error at line 2, got %v", bad, err)
		}
	}
	if _, err := ReadEnvFile(filepath.Join(home, "missing")); err == nil {
		t.Fatal("missing env file must be an error")
	}
}

// A machine joins several hubs: the first stays primary, the file keeps the
// old layout for it, and rejoining a hub replaces only that membership.
func TestMachineSeveralHubs(t *testing.T) {
	dir := t.TempDir()
	def := filepath.Join(dir, "config.yaml")
	m := &Machine{}
	m.Put(MachineHub{Hub: "https://home", Machine: "mac", EnrollToken: "t1", NamePrefix: "mac-"})
	m.Put(MachineHub{Hub: "https://team.example.com/", Machine: "mac", EnrollToken: "t2", NamePrefix: "glenn-mac-"})
	m.Put(MachineHub{Hub: "https://HOME/", Machine: "mac", EnrollToken: "t3", NamePrefix: "mac-"})
	m.Spawn.MaxAccess = "full"
	if err := SaveMachine(def, m); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(MachinePath(def))
	if !strings.HasPrefix(string(b), "hub: https://HOME/") || !strings.Contains(string(b), "others:") {
		t.Fatalf("layout:\n%s", b)
	}
	got, err := LoadMachine(def)
	if err != nil {
		t.Fatal(err)
	}
	hubs := got.Hubs()
	if len(hubs) != 2 || hubs[0].EnrollToken != "t3" || hubs[1].NamePrefix != "glenn-mac-" || got.Spawn.MaxAccess != "full" {
		t.Fatalf("hubs: %+v", got)
	}
	if h, ok := got.For("https://team.example.com"); !ok || h.EnrollToken != "t2" {
		t.Fatalf("For: %+v %v", h, ok)
	}
	if _, ok := got.For("https://other"); ok {
		t.Fatal("unknown hub found")
	}
	// The old single-hub file still reads.
	if err := os.WriteFile(MachinePath(def), []byte("hub: https://old\nmachine: box\nenroll_token: x\nname_prefix: box-\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old, _ := LoadMachine(def)
	if len(old.Hubs()) != 1 || old.Hubs()[0].Machine != "box" {
		t.Fatalf("old layout: %+v", old)
	}
}
