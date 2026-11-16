package config

import (
	"errors"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// ShellAccessFull is the spawn.max_access value that lets agents open a
// terminal (a shell node) on this machine; anything else does not.
const ShellAccessFull = "full"

// ErrNotJoined: this computer has no machine.yaml (it never joined a hub as
// a machine).
var ErrNotJoined = errors.New("this computer has not joined a hub as a machine yet")

// ShellAllowed reports whether machine.yaml next to defaultPath lets agents
// open a terminal here (spawn.max_access: full). A machine that never joined
// does not.
func ShellAllowed(defaultPath string) bool {
	m, err := LoadMachine(defaultPath)
	return err == nil && m != nil && m.Spawn.MaxAccess == ShellAccessFull
}

// SetShellAccess sets spawn.max_access in machine.yaml: full to allow shell
// nodes, back to the default (workspace) to stop allowing new ones. Every
// other key is kept as it is, including ones this version does not know. It
// says whether the file changed; ErrNotJoined when there is none.
func SetShellAccess(defaultPath string, allow bool) (bool, error) {
	path := MachinePath(defaultPath)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, ErrNotJoined
	}
	if err != nil {
		return false, err
	}
	doc := map[string]any{}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return false, err
	}
	spawn, _ := doc["spawn"].(map[string]any)
	if spawn == nil {
		spawn = map[string]any{}
	}
	was := spawn["max_access"] == ShellAccessFull
	if was == allow {
		return false, nil
	}
	if allow {
		spawn["max_access"] = ShellAccessFull
	} else {
		delete(spawn, "max_access") // the default: workspace
	}
	if len(spawn) == 0 {
		delete(doc, "spawn")
	} else {
		doc["spawn"] = spawn
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return false, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}
