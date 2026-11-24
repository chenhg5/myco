package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetSpawnDirs(t *testing.T) {
	dir := t.TempDir()
	def := filepath.Join(dir, "config.yaml")
	home := "/home/alice"
	if _, err := SetSpawnDirs(def, home, []string{"~/code/x"}, nil); err != ErrNotJoined {
		t.Fatalf("no machine.yaml: %v", err)
	}
	yml := "hub: https://hub.example\nspawn:\n  dirs: [\"~/code/demo-team\"]\n  max_access: full\nunknown_key: kept\n"
	if err := os.WriteFile(MachinePath(def), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	ch, err := SetSpawnDirs(def, home, []string{"~/code/acme/", "~/code/demo-team/sub"}, nil)
	if err != nil || !ch.Changed || !reflect.DeepEqual(ch.Dirs, []string{"~/code/demo-team", "~/code/acme"}) || !reflect.DeepEqual(ch.Covered, []string{"~/code/demo-team/sub"}) {
		t.Fatalf("add: %+v %v", ch, err)
	}
	b, _ := os.ReadFile(MachinePath(def))
	if !strings.Contains(string(b), "unknown_key: kept") || !strings.Contains(string(b), "max_access: full") || !strings.Contains(string(b), "~/code/acme") {
		t.Fatalf("machine.yaml:\n%s", b)
	}
	m, err := LoadMachine(def)
	if err != nil || !reflect.DeepEqual(m.Spawn.Dirs, []string{"~/code/demo-team", "~/code/acme"}) {
		t.Fatalf("loaded: %+v %v", m, err)
	}
	// A parent takes the place of the folders inside it.
	if ch, _ := SetSpawnDirs(def, home, []string{"~/code"}, nil); !reflect.DeepEqual(ch.Dirs, []string{"~/code"}) {
		t.Fatalf("parent: %+v", ch)
	}
	// The last one stays: none would mean all of home.
	if _, err := SetSpawnDirs(def, home, nil, []string{"~/code"}); err != ErrLastDir {
		t.Fatalf("last: %v", err)
	}
	if ch, err := SetSpawnDirs(def, home, []string{"/srv/data"}, []string{"/home/alice/code"}); err != nil || !reflect.DeepEqual(ch.Dirs, []string{"/srv/data"}) {
		t.Fatalf("swap: %+v %v", ch, err)
	}
	if ch, _ := SetSpawnDirs(def, home, nil, []string{"~/nope"}); ch.Changed || len(ch.Missing) != 1 {
		t.Fatalf("missing: %+v", ch)
	}
	for _, bad := range []string{"", "/", "code/x", "C:\\"} {
		if _, err := SetSpawnDirs(def, home, []string{bad}, nil); err == nil && !(bad == "C:\\" && filepath.Separator == '/') {
			t.Fatalf("%q was taken", bad)
		}
	}
}

func TestSpawnDirsDefaultIsHome(t *testing.T) {
	dir := t.TempDir()
	def := filepath.Join(dir, "config.yaml")
	os.WriteFile(MachinePath(def), []byte("hub: https://hub.example\n"), 0o600)
	// Home is allowed already: nothing to add.
	ch, err := SetSpawnDirs(def, "/home/g", []string{"~/code/x"}, nil)
	if err != nil || ch.Changed || !reflect.DeepEqual(ch.Dirs, []string{"~"}) || len(ch.Covered) != 1 {
		t.Fatalf("default: %+v %v", ch, err)
	}
}
