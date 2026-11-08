package config

import (
	"encoding/json"
	"github.com/chenhg5/myco/internal/osutil"
	"github.com/chenhg5/myco/internal/testutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMachineNeverJoined(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	m, err := LoadMachine(DefaultPath())
	if m != nil || err != nil {
		t.Fatalf("LoadMachine before join = %+v, %v; want nil, nil", m, err)
	}
}

func TestMachineRoundTripAndPermissions(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	def := DefaultPath()
	in := &Machine{
		Hub: "https://hub", Machine: "vps", EnrollToken: "myco_enroll",
		NamePrefix: "vps-", Harness: "codex", AllowFrom: []string{"alice", "bob"},
	}
	if err := SaveMachine(def, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadMachine(def)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip:\n in=%+v\nout=%+v", in, out)
	}
	path := MachinePath(def)
	if p := perm(t, path); osutil.UnixPermissions && p != 0o600 {
		t.Errorf("machine.yaml mode = %o, want 600 (holds the enroll token)", p)
	}
	if p := perm(t, filepath.Dir(path)); osutil.UnixPermissions && p != 0o700 {
		t.Errorf("config dir mode = %o, want 700", p)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
	// The enroll token is persisted in YAML but never exposed in JSON.
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "enroll_token: myco_enroll") {
		t.Fatalf("machine.yaml missing enroll token:\n%s", b)
	}
	j, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(j), "myco_enroll") {
		t.Fatalf("enroll token leaked in JSON: %s", j)
	}
}

func TestLoadMachineMalformed(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	def := DefaultPath()
	writeFile(t, MachinePath(def), "hub: [x\n")
	if m, err := LoadMachine(def); err == nil || m != nil {
		t.Fatalf("malformed machine.yaml: m=%+v err=%v", m, err)
	}
}
