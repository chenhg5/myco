package osutil

import (
	"path/filepath"
	"testing"
)

func TestWithin(t *testing.T) {
	root := filepath.FromSlash("/home/me")
	for _, c := range []struct {
		p    string
		want bool
	}{
		{"/home/me", true},
		{"/home/me/code", true},
		{"/home/me/", true},
		{"/home/meme", false},
		{"/home", false},
		{"/etc", false},
	} {
		if got := Within(filepath.FromSlash(c.p), root); got != c.want {
			t.Errorf("Within(%q) = %v, want %v", c.p, got, c.want)
		}
	}
	if !Within(filepath.FromSlash("/"), filepath.FromSlash("/")) || !Within(filepath.FromSlash("/x"), filepath.FromSlash("/")) {
		t.Error("everything is within /")
	}
}

func TestExpandHome(t *testing.T) {
	home := filepath.FromSlash("/home/me")
	if got := ExpandHome("~", home); got != home {
		t.Errorf("~ -> %q", got)
	}
	if got, want := ExpandHome("~/code/app", home), filepath.Join(home, "code", "app"); got != want {
		t.Errorf("~/code/app -> %q, want %q", got, want)
	}
	if got := ExpandHome("/abs", home); got != "/abs" {
		t.Errorf("/abs -> %q", got)
	}
	if got := ExpandHome("~other/x", home); got != "~other/x" {
		t.Errorf("~other/x -> %q", got)
	}
}

func TestInCredentialDir(t *testing.T) {
	for p, want := range map[string]bool{
		"/home/me/.ssh/id_ed25519":        true,
		"/home/me/.myco/config.yaml":      true,
		`C:\Users\me\.aws\credentials`:    true,
		"/home/me/code/ssh.md":            false,
		"/home/me/.sshconfig-notes":       false,
		`C:\Users\me\Documents\notes.txt`: false,
	} {
		if got := InCredentialDir(p); got != want {
			t.Errorf("InCredentialDir(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestUserName(t *testing.T) {
	env := map[string]string{"USERNAME": "glenn"}
	if got := UserName(func(k string) string { return env[k] }); got != "glenn" {
		t.Errorf("USERNAME fallback = %q", got)
	}
	env["USER"] = "me"
	if got := UserName(func(k string) string { return env[k] }); got != "me" {
		t.Errorf("USER first = %q", got)
	}
}
