package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/runnerupdate"
)

// LocalUpgrade keeps the detached CLI's receipt across App relaunches. Runtime
// comes from the running runner, never from the replaced executable's file.
type LocalUpgrade = api.LocalUpgradeStatus

var cliUpgradeMu sync.Mutex

func (l *Local) upgradeBase() string { return config.Dir(l.configPath()) }
func (l *Local) upgradeStatus(ctx context.Context) LocalUpgrade {
	out := LocalUpgrade{}
	base := l.upgradeBase()
	now := time.Now()
	var receipt runnerupdate.Receipt
	if runnerupdate.Read(base, "cli", &receipt) == nil {
		out.CLI = &receipt
		if receipt.ID != "" && strings.IndexFunc(receipt.ID, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
			var done runnerupdate.Receipt
			if runnerupdate.Read(base, "result-"+receipt.ID, &done) == nil {
				done.ID = receipt.ID
				out.CLI = &done
			}
		}
	}
	var rt runnerupdate.Runtime
	if runnerupdate.Read(base, "runtime", &rt) == nil && runnerupdate.Fresh(&rt, now) {
		out.Runner = &rt
	}
	if out.Runner == nil {
		// Older runners already publish their running version in slot runtime,
		// but cannot atomically pause claims; never guess that they are safe.
		if old, err := config.LoadRunnerSlots(l.configPath()); err == nil && old.Fresh(now) {
			out.Runner = &runnerupdate.Runtime{PID: old.PID, Version: old.Version, At: old.At}
			out.Unsupported = true
		}
	}
	if req, err := runnerupdate.LoadRequest(base); err == nil {
		out.Restart = req
		out.Expired = !now.Before(req.Deadline)
	}
	call, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	if data, err := l.run(call, "version", "--offline"); err == nil {
		var v struct {
			Version string `json:"version"`
			Client  string `json:"client"`
		}
		if json.Unmarshal(data, &v) == nil {
			out.Installed = orStr(v.Version, v.Client)
		}
	}
	if out.Runner != nil && out.Installed != "" {
		out.Pending = strings.TrimPrefix(out.Installed, "v") != strings.TrimPrefix(out.Runner.Version, "v")
	}
	if out.Restart != nil && (out.Runner == nil || out.Runner.RequestID != out.Restart.ID || out.Runner.State != "complete") {
		out.Pending = true
	}
	if out.CLI != nil && out.CLI.State == "running" && now.Sub(out.CLI.At) > 35*time.Minute {
		copy := *out.CLI
		copy.State = "failed"
		copy.Error = "CLI upgrade did not report completion; retry the upgrade"
		out.CLI = &copy
	}
	if out.Runner != nil {
		out.Error = out.Runner.Error
	}
	return out
}

// upgradeCLIDetached outlives the App. The CLI itself owns the final receipt,
// so the App closing cannot silently discard success or failure.
func (l *Local) upgradeCLIDetached(target string) error {
	cliUpgradeMu.Lock()
	defer cliUpgradeMu.Unlock()
	base := l.upgradeBase()
	var old runnerupdate.Receipt
	if runnerupdate.Read(base, "cli", &old) == nil && old.State == "running" && time.Since(old.At) < 35*time.Minute {
		var final runnerupdate.Receipt
		if old.ID == "" || runnerupdate.Read(base, "result-"+old.ID, &final) != nil {
			return nil
		}
	}
	receipt := runnerupdate.Receipt{ID: fmt.Sprintf("%d", time.Now().UnixNano()), State: "running", Target: target, At: time.Now().UTC()}
	fail := func(err error) error {
		receipt.State = "failed"
		receipt.Error = runnerupdate.SafeMessage(err.Error())
		_ = runnerupdate.Write(base, "cli", receipt)
		return err
	}
	bin := l.mycoPath()
	if bin == "" {
		return fail(fmt.Errorf("the myco command-line tool is not installed"))
	}
	if l.Bundled == "" {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		schema, err := l.run(ctx, "schema", "--all")
		if err != nil || !strings.Contains(string(schema), "restart-when-idle") {
			return fail(fmt.Errorf("this CLI does not support idle restarts yet; upgrade the command-line tool once from a terminal"))
		}
	}
	if err := runnerupdate.Write(base, "cli", receipt); err != nil {
		return err
	}
	// Only the structured receipt is retained; raw stderr can contain private download URLs.
	logPath := os.DevNull
	start := l.StartProcess
	if start == nil {
		start = startDetached
	}
	cmd := []string{bin, "upgrade", "--restart-when-idle", "--result-file", runnerupdate.Path(base, "result-"+receipt.ID), "--format", "json"}
	// The new App's bundled CLI understands the new policy even if the
	// installed CLI predates it. It still replaces the installed path.
	env := append(l.env(), "MYCO_CONFIG="+l.configPath())
	if target != "" {
		cmd = append(cmd, "--version", target)
	}
	if l.Bundled != "" {
		cmd[0] = l.Bundled
		env = append(env, "MYCO_UPGRADE_TARGET="+bin)
	}
	if _, err := start(cmd, logPath, env); err != nil {
		return fail(err)
	}
	return nil
}

// Old runners predate the local runtime file. Their workspace machine report
// can identify the running version, but never authorizes a service restart.
func (b *Backend) localUpgradeStatus(ctx context.Context) LocalUpgrade {
	out := b.Local.upgradeStatus(ctx)
	if out.Runner != nil || b.Store == nil || b.Secrets == nil {
		return out
	}
	local, err := config.LoadMachine(b.Local.configPath())
	if err != nil || local == nil {
		return out
	}
	hubs, err := b.Store.List()
	if err != nil {
		return out
	}
	ctx, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	replies := make(chan *runnerupdate.Runtime, len(hubs))
	pending := 0
	for _, hub := range hubs {
		own, ok := local.For(hub.URL)
		if !ok {
			continue
		}
		c, err := b.client(hub)
		if err != nil {
			continue
		}
		pending++
		go func() {
			var found *runnerupdate.Runtime
			defer func() { replies <- found }()
			machines, err := c.Machines(ctx)
			if err != nil {
				return
			}
			for _, m := range machines {
				match := own.Machine != "" && (m.ID == own.Machine || m.Node == "" && m.Name == own.Machine) || own.NamePrefix != "" && m.Node == own.NamePrefix+"machine"
				if !match || !m.Online || m.MycoVersion == "" || m.LastSeenAt == nil {
					continue
				}
				found = &runnerupdate.Runtime{Version: m.MycoVersion, At: *m.LastSeenAt}
				return
			}
		}()
	}
	for range pending {
		if rt := <-replies; rt != nil && (out.Runner == nil || rt.At.After(out.Runner.At)) {
			out.Runner = rt
		}
	}
	if out.Runner != nil {
		out.Unsupported = true
		if out.Installed != "" {
			out.Pending = strings.TrimPrefix(out.Installed, "v") != strings.TrimPrefix(out.Runner.Version, "v")
		}
	}
	return out
}

func (b *Backend) getLocalUpgrade(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, b.localUpgradeStatus(r.Context()))
}
func (b *Backend) postLocalUpgrade(w http.ResponseWriter, r *http.Request) {
	if err := b.Local.upgradeCLIDetached(b.Version); err != nil {
		writeErr(w, &client.Error{Status: 502, Code: "upgrade_failed", Message: runnerupdate.SafeMessage(err.Error()), Suggestion: "retry the command-line update", Retryable: true})
		return
	}
	writeJSON(w, http.StatusAccepted, b.localUpgradeStatus(r.Context()))
}
func (b *Backend) postIdleRestart(w http.ResponseWriter, r *http.Request) {
	st := b.localUpgradeStatus(r.Context())
	if st.Unsupported {
		writeErr(w, &client.Error{Status: 409, Code: "unsupported", Message: "the running runner predates idle restart support", Suggestion: "after every task finishes, restart the runner once from a terminal"})
		return
	}
	if st.Installed == "" {
		writeErr(w, &client.Error{Status: 409, Code: "unavailable", Message: "cannot read the installed CLI version", Suggestion: "retry the CLI upgrade", Retryable: true})
		return
	}
	now := time.Now().UTC()
	req := runnerupdate.Request{ID: fmt.Sprintf("%d", now.UnixNano()), Target: st.Installed, RequestedAt: now, Deadline: now.Add(runnerupdate.IdleLimit)}
	if err := runnerupdate.Write(b.Local.upgradeBase(), "request", req); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, b.localUpgradeStatus(r.Context()))
}
