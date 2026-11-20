package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
)

// Which folders agents may start in on a computer (spawn.dirs in its
// machine.yaml) is that computer's own policy: no hub changes it. The App
// changes it for the person when it reaches that computer itself: this
// computer (the file here), or one it connected over SSH or into WSL (it
// runs `myco machine dirs` there). For any other computer it gives the
// command to run on it.
//
//	POST /app/hubs/{id}/machines/{mid}/dirs {add?, remove?, here?}
//	  → {dirs, changed, already_allowed?, not_listed?, via: here|ssh|wsl}
//	  409 not_reachable {suggestion: the command to run on it}

// DirsChange is the answer of a dirs change.
type DirsChange struct {
	Dirs           []string `json:"dirs"`
	Changed        bool     `json:"changed"`
	AlreadyAllowed []string `json:"already_allowed,omitempty"`
	NotListed      []string `json:"not_listed,omitempty"`
	Via            string   `json:"via"`
}

// dirsCommand is the command that makes the change on that computer.
func dirsCommand(add, remove []string) string {
	parts := []string{"myco machine dirs"}
	for _, d := range add {
		parts = append(parts, "--add "+shellQuote(d))
	}
	for _, d := range remove {
		parts = append(parts, "--remove "+shellQuote(d))
	}
	return strings.Join(parts, " ")
}

// setLocalDirs changes spawn.dirs in this computer's machine.yaml.
func (l *Local) setLocalDirs(add, remove []string) (*DirsChange, error) {
	for i, a := range add {
		d, err := config.CleanSpawnDir(a)
		if err != nil {
			return nil, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error()}
		}
		if st, err := os.Stat(config.ExpandDir(d, l.Home)); err != nil || !st.IsDir() {
			return nil, &client.Error{Status: http.StatusNotFound, Code: "not_found", Message: "folder " + d + " does not exist on this computer",
				Suggestion: "create it first, or check the name"}
		}
		add[i] = d
	}
	ch, err := config.SetSpawnDirs(l.configPath(), l.Home, add, remove)
	switch {
	case errors.Is(err, config.ErrNotJoined):
		return nil, &client.Error{Status: http.StatusConflict, Code: "not_configured", Message: "this computer has not joined a hub as a machine yet"}
	case errors.Is(err, config.ErrLastDir):
		return nil, &client.Error{Status: http.StatusConflict, Code: "invalid_state", Message: err.Error(), Suggestion: "allow another folder first, then remove this one"}
	case err != nil:
		return nil, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error()}
	}
	return &DirsChange{Dirs: ch.Dirs, Changed: ch.Changed, AlreadyAllowed: ch.Covered, NotListed: ch.Missing, Via: "here"}, nil
}

// setRemoteDirs runs `myco machine dirs` on the computer rt reaches.
func (l *Local) setRemoteDirs(ctx context.Context, rt Route, add, remove []string) (*DirsChange, error) {
	script := `cd "$HOME" 2>/dev/null
M="$(command -v myco 2>/dev/null)"; [ -z "$M" ] && M="$HOME/.local/bin/myco"
"$M"` + strings.TrimPrefix(dirsCommand(add, remove), "myco") + "\n"
	var out []string
	stderr, code, err := l.runScript(ctx, rt, script, func(s string) { out = append(out, s) })
	if err != nil {
		return nil, err
	}
	if code != 0 {
		var e struct {
			Code       string `json:"error"`
			Message    string `json:"message"`
			Suggestion string `json:"suggestion"`
		}
		msg := strings.TrimSpace(stderr)
		if strings.Contains(msg, "unknown command") {
			return nil, &client.Error{Status: http.StatusConflict, Code: "too_old", Message: "the myco on that computer is too old to change its folders",
				Suggestion: "upgrade it (电脑页「升级」), then try again"}
		}
		if i := strings.Index(msg, "{"); i >= 0 && json.Unmarshal([]byte(msg[i:]), &e) == nil && e.Message != "" {
			return nil, &client.Error{Status: http.StatusConflict, Code: e.Code, Message: e.Message, Suggestion: e.Suggestion}
		}
		return nil, &client.Error{Status: http.StatusBadGateway, Code: "failed", Message: orStr(lastLines(msg, 3), "it did not work"), Suggestion: dirsCommand(add, remove)}
	}
	var got struct {
		Dirs           []string `json:"dirs"`
		Changed        bool     `json:"changed"`
		AlreadyAllowed []string `json:"already_allowed"`
		NotListed      []string `json:"not_listed"`
	}
	if err := json.Unmarshal([]byte(strings.Join(out, "\n")), &got); err != nil {
		return nil, &client.Error{Status: http.StatusBadGateway, Code: "failed", Message: "its answer could not be read", Suggestion: dirsCommand(add, remove)}
	}
	return &DirsChange{Dirs: got.Dirs, Changed: got.Changed, AlreadyAllowed: got.AlreadyAllowed, NotListed: got.NotListed, Via: rt.Via}, nil
}

func (b *Backend) postMachineDirs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
		// Here: the machine is this computer (the App knows it by its
		// workspace's machine id).
		Here bool `json:"here"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Add)+len(in.Remove) == 0 || len(in.Add)+len(in.Remove) > 20 {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "name the folders to add or remove"})
		return
	}
	if in.Here {
		out, err := b.Local.setLocalDirs(in.Add, in.Remove)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	rt, ok := b.reachOf(r.PathValue("id"), r.PathValue("mid"))
	if !ok {
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "not_reachable", Message: "only that computer can change its folders, and this App does not reach it",
			Suggestion: dirsCommand(in.Add, in.Remove)})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	out, err := b.Local.setRemoteDirs(ctx, rt, in.Add, in.Remove)
	if err != nil {
		var re *RemoteError
		if errors.As(err, &re) {
			writeRemoteErr(w, err, rt)
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
