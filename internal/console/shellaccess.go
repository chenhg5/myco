package console

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
)

// Shell nodes run the commands of the agents they allow with this user's
// full rights, so a machine only adds one when its own machine.yaml says
// spawn.max_access: full. Nobody can turn that on remotely; the app does it
// here, on this computer, after the person confirms.

func (l *Local) configPath() string { return filepath.Join(l.Home, ".myco", "config.yaml") }

// localMachine is this computer's machine names (one per hub it joined) and
// whether it allows shell nodes.
func (l *Local) localMachine() (names []string, shellAllowed bool) {
	m, err := config.LoadMachine(l.configPath())
	if err != nil || m == nil {
		return nil, false
	}
	for _, h := range m.Hubs() {
		if h.Machine != "" {
			names = append(names, h.Machine)
		}
	}
	return names, m.Spawn.MaxAccess == string(harness.AccessFull)
}

// setShellAccess sets spawn.max_access in machine.yaml: full to allow shell
// nodes, back to the default (workspace) to stop allowing new ones. Every
// other key is kept as it is, including ones this app does not know.
func (l *Local) setShellAccess(allow bool) error {
	path := config.MachinePath(l.configPath())
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &client.Error{Status: http.StatusConflict, Code: "not_configured", Message: "this computer has not joined a hub as a machine yet",
			Suggestion: "add this computer on the Machines page first"}
	}
	if err != nil {
		return err
	}
	doc := map[string]any{}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return err
	}
	spawn, _ := doc["spawn"].(map[string]any)
	if spawn == nil {
		spawn = map[string]any{}
	}
	switch {
	case allow:
		spawn["max_access"] = string(harness.AccessFull)
	case spawn["max_access"] == string(harness.AccessFull):
		delete(spawn, "max_access") // the default: workspace
	}
	if len(spawn) == 0 {
		delete(doc, "spawn")
	} else {
		doc["spawn"] = spawn
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// postShellAccess is POST /app/local/shell-access {"allow": bool}.
func (b *Backend) postShellAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Allow bool `json:"allow"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := b.Local.setShellAccess(in.Allow); err != nil {
		writeErr(w, err)
		return
	}
	_, allowed := b.Local.localMachine()
	writeJSON(w, http.StatusOK, map[string]bool{"shell_allowed": allowed})
}
