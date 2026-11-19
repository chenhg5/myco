package console

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
)

// Shell nodes run the commands of the agents they allow with this user's
// full rights, so a machine only adds one when its own machine.yaml says
// spawn.max_access: full. Nobody can turn that on remotely; the app does it
// here, on this computer, after the person confirms.

func (l *Local) configPath() string { return filepath.Join(l.Home, ".myco", "config.yaml") }

// localMachine is this computer's machine names (one per hub it joined) and
// whether it allows shell nodes.
func (l *Local) localMachine() (names, nodes []string, shellAllowed bool) {
	m, err := config.LoadMachine(l.configPath())
	if err != nil || m == nil {
		return nil, nil, false
	}
	for _, h := range m.Hubs() {
		if h.Machine != "" {
			names = append(names, h.Machine)
		}
		if h.NamePrefix != "" {
			// Its machine node there: <prefix>machine (machine_node.go).
			nodes = append(nodes, h.NamePrefix+"machine")
		}
	}
	return names, nodes, m.Spawn.MaxAccess == config.ShellAccessFull
}

// setShellAccess sets spawn.max_access in machine.yaml (config.SetShellAccess):
// full to allow shell nodes, back to the default (workspace) to stop allowing
// new ones.
func (l *Local) setShellAccess(allow bool) error {
	_, err := config.SetShellAccess(l.configPath(), allow)
	if errors.Is(err, config.ErrNotJoined) {
		return &client.Error{Status: http.StatusConflict, Code: "not_configured", Message: "this computer has not joined a hub as a machine yet",
			Suggestion: "add this computer on the Machines page first"}
	}
	return err
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
	_, _, allowed := b.Local.localMachine()
	writeJSON(w, http.StatusOK, map[string]bool{"shell_allowed": allowed})
}
