// Package codex adapts OpenAI Codex CLI's non-interactive mode (`codex exec`).
package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// Name is the registry key.
const Name = "codex"

type codex struct {
	opts harness.Options
	*harness.TranscriptFinder
}

// New is the harness.Factory for Codex.
func New(opts harness.Options) (harness.Harness, error) {
	if opts.Binary == "" {
		opts.Binary = "codex"
	}
	return &codex{opts: opts, TranscriptFinder: &harness.TranscriptFinder{Patterns: transcriptPatterns}}, nil
}

// transcriptPatterns: $CODEX_HOME/sessions/YYYY/MM/DD/rollout-<ts>-<id>.jsonl
func transcriptPatterns(id string) []string {
	return []string{filepath.Join(harness.HomeDir("CODEX_HOME", ".codex"), "sessions", "*", "*", "*", "rollout-*"+id+".jsonl")}
}

func (c *codex) Name() string { return Name }

// accessArgs maps access levels to Codex flags. `exec resume` rejects -s,
// so the sandbox is set through -c, which both forms accept.
var accessArgs = map[harness.Access][]string{
	harness.AccessReadonly: {"-c", `sandbox_mode="read-only"`},
	// Network on so the agent can reach the hub with `myco` (reply, hold,
	// delegate); file writes stay confined to the workspace.
	harness.AccessWorkspace: {"-c", `sandbox_mode="workspace-write"`, "-c", "sandbox_workspace_write.network_access=true"},
	harness.AccessFull:      {"--dangerously-bypass-approvals-and-sandbox"},
}

// Check implements harness.Checker.
func (c *codex) Check(ctx context.Context) harness.Check {
	return harness.CheckBinary(ctx, c.opts.Binary, "--version")
}

// event is the subset of `codex exec --json` JSONL events we use.
type event struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Message  string `json:"message"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"`
	Item struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Message string `json:"message"`
	} `json:"item"`
}

// CanFork implements harness.Forker (codex exec fork <id>).
func (c *codex) CanFork() {}

// AppliesProvider implements harness.ProviderAware (-c overrides).
func (c *codex) AppliesProvider() {}

func (c *codex) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	dir, err := os.MkdirTemp("", "myco-codex-")
	if err != nil {
		return harness.Result{}, err
	}
	defer os.RemoveAll(dir)
	lastMsg := filepath.Join(dir, "last.txt")

	// codex exec [flags] [resume <id>] -   (prompt read from stdin)
	args := []string{"exec", "--json", "--output-last-message", lastMsg}
	model := req.Model
	if model == "" {
		model = c.opts.Model
	}
	if model != "" {
		args = append(args, "-m", model)
	}
	args = append(args, accessArgs[c.opts.Access]...)
	env := req.Env
	if p := req.Provider; p != nil {
		// -c overrides beat ~/.codex/config.toml for this run only; the key
		// travels in the environment (the provider's env_key).
		args = append(args, p.ConfigArgs...)
		env = append(p.EnvList(), env...)
	}
	args = append(args, c.opts.Args...)
	// After the runner's own args: the effort chosen in the app wins over
	// a model_reasoning_effort set there (the last -c of a key wins).
	if req.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+strconv.Quote(req.Effort))
	}
	switch {
	case req.SessionID != "" && req.Fork:
		args = append(args, "fork", req.SessionID)
	case req.SessionID != "":
		args = append(args, "resume", req.SessionID)
	}
	args = append(args, "-")

	proc := harness.Process{
		Binary: c.opts.Binary, Args: args, Stdin: req.Prompt, Workdir: req.Workdir, Env: env,
	}
	if req.OnEvent != nil {
		before, known := tokenTotal{}, true
		if req.SessionID != "" {
			// A resumed (or copied) session: Codex reports its running total.
			before, known = sessionTokens(req.SessionID)
		}
		stream := newEventStream(time.Now(), before, known)
		proc.OnStdoutLine = func(line []byte) {
			for _, ev := range stream.line(line) {
				req.OnEvent(ev)
			}
		}
	}
	out, runErr := harness.Exec(ctx, proc)
	res, failure := parseEvents(out.Stdout)
	if b, err := os.ReadFile(lastMsg); err == nil && strings.TrimSpace(string(b)) != "" {
		res.Output = strings.TrimSpace(string(b))
	}
	if req.SessionID != "" && res.SessionID == "" && !req.Fork {
		res.SessionID = req.SessionID
	}
	// The account's limits, as this run left them in the session file.
	if req.OnEvent != nil && res.SessionID != "" {
		if q := sessionQuota(res.SessionID); q != nil {
			req.OnEvent(harness.Event{Kind: "quota", Quota: q})
		}
	}
	if runErr != nil && strings.Contains(runErr.Error()+failure, "already has an active writer") {
		return res, fmt.Errorf("%w: codex thread %s is open elsewhere (interactive session?)", harness.ErrSessionBusy, req.SessionID)
	}
	if runErr != nil && failure != "" {
		// The real cause is in the JSON events; stderr is mostly warnings.
		return res, fmt.Errorf("codex failed: %s", failure)
	}
	return res, runErr
}

// parseEvents extracts the session id, the final agent message and, if the
// run failed, the most specific error in the event stream: turn.failed beats
// error events, which beat non-fatal "error" items (Codex also uses those for
// warnings such as missing model metadata).
func parseEvents(stdout string) (harness.Result, string) {
	var (
		res                         harness.Result
		turnFailed, errs, itemNotes []string
	)
	sc := bufio.NewScanner(strings.NewReader(stdout))
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var ev event
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch {
		case ev.Type == "thread.started" && ev.ThreadID != "":
			res.SessionID = ev.ThreadID
		case ev.Type == "item.completed" && ev.Item.Type == "agent_message":
			res.Output = ev.Item.Text
		case ev.Type == "turn.failed" && ev.Error != nil && ev.Error.Message != "":
			turnFailed = append(turnFailed, unwrapAPIError(ev.Error.Message))
		case ev.Type == "error" && ev.Message != "":
			errs = append(errs, unwrapAPIError(ev.Message))
		case ev.Type == "item.completed" && ev.Item.Type == "error" && ev.Item.Message != "":
			itemNotes = append(itemNotes, ev.Item.Message)
		}
	}
	for _, group := range [][]string{turnFailed, errs, itemNotes} {
		if len(group) > 0 {
			return res, strings.Join(dedupe(group), "; ")
		}
	}
	return res, ""
}

// unwrapAPIError turns `{"error":{"message":"..."}}` payloads into the
// human-readable message.
func unwrapAPIError(s string) string {
	var v struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(s), &v) == nil && v.Error.Message != "" {
		if v.Error.Code != "" {
			return v.Error.Message + " (" + v.Error.Code + ")"
		}
		return v.Error.Message
	}
	return s
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// tailBytes is how much of a session file is read for its token total.
const tailBytes = 4 << 20

// sessionTokens is a session's token total so far, from the last
// token_count in its rollout file; false when it cannot be told.
func sessionTokens(id string) (tokenTotal, bool) {
	var path string
	for _, pat := range transcriptPatterns(id) {
		if m, _ := filepath.Glob(pat); len(m) > 0 {
			path = m[0]
			break
		}
	}
	if path == "" {
		return tokenTotal{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return tokenTotal{}, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return tokenTotal{}, false
	}
	whole := st.Size() <= tailBytes
	if !whole {
		if _, err := f.Seek(-tailBytes, io.SeekEnd); err != nil {
			return tokenTotal{}, false
		}
	}
	var total tokenTotal
	found := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, []byte(`"token_count"`)) {
			continue
		}
		var ev struct {
			Payload struct {
				Info *struct {
					Total tokenTotal `json:"total_token_usage"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &ev) == nil && ev.Payload.Info != nil {
			total, found = ev.Payload.Info.Total, true
		}
	}
	if found {
		return total, true
	}
	// No count yet: a session without a turn starts from zero (when the
	// whole file was read).
	return tokenTotal{}, whole
}

// rateLimits is the rate_limits of a token_count event in a Codex session
// file, e.g. {"primary":{"used_percent":7.0,"window_minutes":10080,
// "resets_at":1794735584},"secondary":null,"rate_limit_reached_type":null}.
type rateLimits struct {
	Primary     *rateWindow `json:"primary"`
	Secondary   *rateWindow `json:"secondary"`
	ReachedType *string     `json:"rate_limit_reached_type"`
}

type rateWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int64   `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
	// Older Codex versions said how long until the reset instead.
	ResetsInSeconds int64 `json:"resets_in_seconds"`
}

// sessionQuota is the latest rate_limits in a session's file, nil if none.
func sessionQuota(id string) *harness.Quota {
	var path string
	for _, pat := range transcriptPatterns(id) {
		if m, _ := filepath.Glob(pat); len(m) > 0 {
			path = m[0]
			break
		}
	}
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > tailBytes {
		if _, err := f.Seek(-tailBytes, io.SeekEnd); err != nil {
			return nil
		}
	}
	var last *rateLimits
	var at time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, []byte(`"rate_limits"`)) {
			continue
		}
		var ev struct {
			Timestamp time.Time `json:"timestamp"`
			Payload   struct {
				RateLimits *rateLimits `json:"rate_limits"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &ev) == nil && ev.Payload.RateLimits != nil {
			last, at = ev.Payload.RateLimits, ev.Timestamp
		}
	}
	return last.quota(at)
}

func (r *rateLimits) quota(at time.Time) *harness.Quota {
	if r == nil {
		return nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	q := &harness.Quota{Source: "codex"}
	for _, w := range []struct {
		name string
		win  *rateWindow
	}{{"primary", r.Primary}, {"secondary", r.Secondary}} {
		if w.win == nil {
			continue
		}
		qw := harness.QuotaWindow{Name: w.name, Label: windowLabel(w.win.WindowMinutes), Used: w.win.UsedPercent / 100}
		switch {
		case w.win.ResetsAt > 0:
			qw.ResetsAt = time.Unix(w.win.ResetsAt, 0).UTC()
		case w.win.ResetsInSeconds > 0:
			qw.ResetsAt = at.Add(time.Duration(w.win.ResetsInSeconds) * time.Second).UTC()
		}
		if r.ReachedType != nil && *r.ReachedType != "" && qw.Used >= 1 {
			qw.Status = *r.ReachedType
		}
		q.Windows = append(q.Windows, qw)
	}
	if len(q.Windows) == 0 {
		return nil
	}
	return q
}

// windowLabel names a window by its length: 5h, 7d.
func windowLabel(minutes int64) string {
	switch {
	case minutes <= 0:
		return ""
	case minutes%(24*60) == 0:
		return strconv.FormatInt(minutes/(24*60), 10) + "d"
	case minutes%60 == 0:
		return strconv.FormatInt(minutes/60, 10) + "h"
	}
	return strconv.FormatInt(minutes, 10) + "m"
}
