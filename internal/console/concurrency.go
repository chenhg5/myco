package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
)

// How many agent CLIs a computer's runner runs at once (runner start --all,
// the background service). This computer's is runner.max_concurrent in its
// machine.yaml, which its runner reads within seconds; another computer's
// goes through its machine node.

// LocalConcurrency is GET /app/local/concurrency.
type LocalConcurrency struct {
	// Setting is runner.max_concurrent in machine.yaml (0: not set).
	Setting int `json:"setting"`
	// Runner is what the runner running here does; nil when none says it
	// follows the setting (none running, or one older than it: restart it).
	Runner  *api.MachineConcurrency `json:"runner"`
	Default int                     `json:"default"`
	Ceiling int                     `json:"ceiling"`
	// Joined: this computer has a machine.yaml (the setting can be written).
	Joined bool `json:"joined"`
}

func (l *Local) concurrency(now time.Time) LocalConcurrency {
	out := LocalConcurrency{Default: config.DefaultMaxConcurrent, Ceiling: config.MaxConcurrentCeiling}
	if m, err := config.LoadMachine(l.configPath()); err == nil && m != nil {
		out.Joined, out.Setting = true, m.Runner.MaxConcurrent
	}
	if st, err := config.LoadRunnerSlots(l.configPath()); err == nil && st.Fresh(now) {
		out.Runner = &api.MachineConcurrency{Limit: st.Limit, InUse: st.InUse, Waiting: st.Waiting, Source: st.Source, Flag: st.Flag, Ceiling: config.MaxConcurrentCeiling}
	}
	return out
}

func (b *Backend) getLocalConcurrency(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, b.Local.concurrency(time.Now()))
}

// postLocalConcurrency is POST /app/local/concurrency {"max_concurrent": n}:
// written now, so it wins over the runner's flag; the runner applies it
// within seconds (raising starts waiting tasks, lowering only stops new
// starts). The answer is the state right after writing: Runner shows the
// new limit once the runner has read it (ask again).
func (b *Backend) postLocalConcurrency(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MaxConcurrent int `json:"max_concurrent"`
	}
	if !decode(w, r, &in) {
		return
	}
	err := config.SetRunnerConcurrency(b.Local.configPath(), in.MaxConcurrent, time.Now())
	switch {
	case errors.Is(err, config.ErrNotJoined):
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "not_configured", Message: "this computer has not joined a hub as a machine yet",
			Suggestion: "add this computer on the Computers page first"})
		return
	case err != nil && strings.Contains(err.Error(), "between"):
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error()})
		return
	case err != nil:
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b.Local.concurrency(time.Now()))
}

// machineConcurrency asks a computer's machine node how many agent CLIs it
// runs at once, after setting it to set when that is not 0. Older
// machines cannot say (too_old: upgrade myco there); a hub older than the
// setting passes only the question on (hub_too_old).
func (b *Backend) machineConcurrency(w http.ResponseWriter, r *http.Request, c *client.Client, set int) {
	res, err := c.SpawnOnMachine(r.Context(), r.PathValue("mid"), api.SpawnRequest{CLI: api.ConcurrencyCLI, MaxConcurrent: set}, 40*time.Second)
	if err != nil {
		if e, ok := client.AsError(err); ok && strings.Contains(e.Message, `cannot spawn "concurrency"`) {
			writeErr(w, tooOldForConcurrency())
			return
		}
		writeErr(w, err)
		return
	}
	if res.Concurrency == nil && res.CLI == api.ConcurrencyCLI && res.Reply != "" {
		// A hub older than it passes the answer on only as text.
		var v api.MachineConcurrency
		if json.Unmarshal([]byte(res.Reply), &v) == nil && v.Limit > 0 {
			res.Concurrency = &v
		}
	}
	if res.Concurrency == nil {
		writeErr(w, tooOldForConcurrency())
		return
	}
	if set != 0 && res.Concurrency.Limit != set {
		// The hub dropped the new number (it predates it): nothing changed.
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "hub_too_old",
			Message: "this workspace's myco is too old to change it from the App", Suggestion: "upgrade the workspace's myco, or run myco machine concurrency --set on that computer"})
		return
	}
	writeJSON(w, http.StatusOK, res.Concurrency)
}

// setMachineConcurrency is POST …/machines/{mid}/concurrency {"max_concurrent": n}.
func (b *Backend) setMachineConcurrency(w http.ResponseWriter, r *http.Request, c *client.Client) {
	var in struct {
		MaxConcurrent int `json:"max_concurrent"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.MaxConcurrent < 1 || in.MaxConcurrent > config.MaxConcurrentCeiling {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: fmt.Sprintf("the limit must be between 1 and %d", config.MaxConcurrentCeiling)})
		return
	}
	b.machineConcurrency(w, r, c, in.MaxConcurrent)
}

func tooOldForConcurrency() error {
	return &client.Error{Status: http.StatusConflict, Code: "too_old",
		Message: "that computer's myco is too old to change how many agent CLIs it runs at once", Suggestion: "upgrade myco on that computer"}
}
