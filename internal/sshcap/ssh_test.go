package sshcap

import (
	"context"
	"github.com/chenhg5/myco/internal/testsshd"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func isolated(t *testing.T) (*Manager, Create) {
	home, config := testsshd.Start(t)
	return &Manager{Home: home, Config: config}, Create{Name: "Fixture Linux", Destination: "fixture"}
}

func TestRealOpenSSHImmutableTarget(t *testing.T) {
	m, in := isolated(t)
	ctx := context.Background()
	p, err := m.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Run(ctx, p.ID, p.Binding, Operation{Action: "probe"}, false)
	if err != nil || strings.TrimSpace(r.Stdout) != "Linux" {
		st, _ := m.load(p.ID)
		f := filepath.Join(m.Home, "frozen")
		os.WriteFile(f, []byte(st.Config), 0600)
		cmd := exec.Command("ssh", "-G", "-F", f, "fixture")
		_, e := cmd.Output()
		if x, ok := e.(*exec.ExitError); ok {
			t.Logf("frozen config rejected: %s", x.Stderr)
		}
		t.Fatalf("real ssh probe: %+v %v", r, err)
	}
	r, err = m.Run(ctx, p.ID, p.Binding, Operation{Action: "overview"}, false)
	if err != nil || !strings.Contains(r.Stdout, "@@myco:memory") || !strings.Contains(r.Stdout, "MemTotal:") {
		t.Fatalf("real Linux overview: %+v %v", r, err)
	}
	if _, err := m.Run(ctx, p.ID, p.Binding, Operation{Action: "probe"}, true); err == nil {
		t.Fatal("agent access default allowed")
	}
	if _, err := m.SetAgent(ctx, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run(ctx, p.ID, "different-target", Operation{Action: "overview"}, true); err == nil {
		t.Fatal("different target accepted")
	}
	profiles, err := (&Manager{Home: m.Home, Config: m.Config}).List()
	if err != nil || len(profiles) != 1 || profiles[0].ID != p.ID {
		t.Fatal("profile id not stable across restart")
	}
	st, _ := os.Stat(filepath.Join(m.root(), p.ID+".json"))
	if st.Mode().Perm() != 0600 {
		t.Fatal("profile not private")
	}
	// Trust/config changes require a new profile and a new human approval.
	f, _ := os.OpenFile(m.Config, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("\n HostKeyAlias another-server\n")
	f.Close()
	if _, err := m.Run(ctx, p.ID, p.Binding, Operation{Action: "probe"}, true); err == nil {
		t.Fatal("changed actual SSH target accepted")
	}
}
func TestRealOpenSSHRejectsUnknownHostKey(t *testing.T) {
	m, in := isolated(t)
	known := filepath.Join(m.Home, "known_hosts")
	b, _ := os.ReadFile(known)
	os.WriteFile(known, []byte("unrelated-host "+strings.Join(strings.Fields(string(b))[1:], " ")+"\n"), 0600)
	p, err := m.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run(context.Background(), p.ID, p.Binding, Operation{Action: "probe"}, false); err == nil {
		t.Fatal("unknown host key trusted")
	}
	if _, err := m.SetAgent(context.Background(), p.ID, true); err == nil {
		t.Fatal("unverified host published to agents")
	}
}
func TestFixedCommandsRejectShellAndOptionInjection(t *testing.T) {
	for _, name := range []string{"-x", "x;touch /tmp/no", "x\nwhoami", "$(id)", "x'", "../x", "reboot.target", "test.socket"} {
		if _, _, err := Command(Operation{Action: "services.restart", Name: name}); err == nil {
			t.Fatal("unsafe resource accepted")
		}
	}
	if _, _, err := Command(Operation{Action: "shell", Name: "id"}); err == nil {
		t.Fatal("arbitrary shell offered")
	}
	if _, _, err := Command(Operation{Action: "services.logs", Name: "ssh.service", Lines: 201}); err == nil {
		t.Fatal("unbounded logs accepted")
	}
}
