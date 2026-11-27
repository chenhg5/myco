//go:build unix

package processwatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryOnlyRecoversExactCreationIdentities(t *testing.T) {
	for _, kind := range []string{"orphan", "live-runner", "pid-reused"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			saved := savedRegistry{Runner: Process{PID: 100, Birth: "old-runner"}, Scopes: []savedScope{{Owner: Owner{Workspace: "workspace", Member: "owner", Agent: "agent"}, Root: Process{PID: 40, Birth: "root"}, Members: []Process{{PID: 50, Birth: "child"}}}}}
			data, _ := json.Marshal(saved)
			old := filepath.Join(dir, "old.json")
			if e := os.WriteFile(old, data, 0600); e != nil {
				t.Fatal(e)
			}
			m := New("new")
			m.stateDir = dir
			m.lookup = func(pid int) (Process, error) {
				switch {
				case pid == os.Getpid():
					return Process{PID: pid, Birth: "new-runner"}, nil
				case pid == 100 && kind == "live-runner":
					return saved.Runner, nil
				case pid == 50:
					birth := "child"
					if kind == "pid-reused" {
						birth = "replacement"
					}
					return Process{PID: pid, Birth: birth}, nil
				}
				return Process{}, ErrIdentity
			}
			m.restore()
			if kind == "orphan" {
				if len(m.scopes) != 1 || m.previous[50].scope.owner.Agent != "agent" {
					t.Fatal("did not restore exact identity")
				}
				info, e := os.Stat(filepath.Join(dir, "new.json"))
				if e != nil || info.Mode().Perm() != 0600 {
					t.Fatal("registry permissions", e)
				}
				if _, e = os.Stat(old); !os.IsNotExist(e) {
					t.Fatal("adopted registry was not cleaned", e)
				}
			} else if len(m.scopes) != 0 {
				t.Fatal("claimed live/reused processes")
			}
			if kind == "live-runner" {
				if _, e := os.Stat(old); e != nil {
					t.Fatal("removed another live runner's registry")
				}
			}
		})
	}
}
