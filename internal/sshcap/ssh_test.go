package sshcap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/testsshd"
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
	// A user@alias command-line destination overrides a config-file User.
	// Freeze the resolved -l override on the execution command line too.
	override, er := m.Create(ctx, Create{Name: "User override", Destination: "not-a-real-user@fixture", User: p.User})
	if er != nil {
		t.Fatal(er)
	}
	if override.User != p.User {
		t.Fatal("resolved user differs from approval")
	}
	if got, er := m.Run(ctx, override.ID, override.Binding, Operation{Action: "probe"}, false); er != nil || strings.TrimSpace(got.Stdout) != "Linux" {
		t.Fatalf("user@alias retargeted the resolved account: %v", er)
	}
	if er := m.Delete(override.ID); er != nil {
		t.Fatal(er)
	}
	r, err = m.Run(ctx, p.ID, p.Binding, Operation{Action: "overview"}, false)
	if err != nil || !strings.Contains(r.Stdout, "@@myco:memory") || !strings.Contains(r.Stdout, "MemTotal:") {
		t.Fatalf("real Linux overview: %+v %v", r, err)
	}
	identity, _, ok := strings.Cut(strings.TrimPrefix(r.Stdout, "@@myco:identity\n"), "@@myco:os\n")
	hostname, er := os.Hostname()
	if er != nil {
		t.Fatal(er)
	}
	if !ok || !strings.Contains(identity, "hostname="+hostname+"\n") {
		t.Fatal("real SSH overview lacks the remote hostname")
	}
	expectedCPUs, er := exec.Command("getconf", "_NPROCESSORS_ONLN").Output()
	if er != nil {
		t.Fatal(er)
	}
	if !strings.Contains(identity, "cpus="+strings.TrimSpace(string(expectedCPUs))+"\n") {
		t.Fatal("real SSH overview lacks the online CPU count")
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

func TestRealOpenSSHRefusesUnfrozenJumpConfiguration(t *testing.T) {
	m, in := isolated(t)
	f, err := os.OpenFile(m.Config, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\n ProxyJump another-host\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Create(context.Background(), in)
	if e, ok := err.(*Error); !ok || e.Code != "unsupported" {
		t.Fatalf("unfrozen SSH hop accepted: %v", err)
	}
}

func TestOverviewIdentityDoesNotInventCPUCount(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux overview script")
	}
	for _, fixture := range []struct{ name, body string }{{"zero", "printf 0"}, {"unavailable", "exit 1"}} {
		t.Run(fixture.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "getconf"), []byte("#!/bin/sh\n"+fixture.body+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/sh", "-c", overview)
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			identity, _, ok := strings.Cut(strings.TrimPrefix(string(output), "@@myco:identity\n"), "@@myco:os\n")
			if !ok || !strings.Contains(identity, "\ncpus=\n") {
				t.Fatal("missing CPU count was not preserved as unknown")
			}
		})
	}
}
