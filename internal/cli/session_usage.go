package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

type usageCursor struct {
	File   string    `json:"file"`
	Offset int64     `json:"offset"`
	LastID string    `json:"last_id"`
	Since  time.Time `json:"since"`
}
type usagePending struct {
	Next    usageCursor       `json:"next"`
	Reports []api.UsageReport `json:"reports"`
	Hub     string            `json:"hub"`
	Agent   string            `json:"agent"`
}
type sessionUsageState struct {
	usageCursor
	Project string        `json:"project"`
	Hub     string        `json:"hub"`
	Agent   string        `json:"agent"`
	Pending *usagePending `json:"pending,omitempty"`
}
type sessionUsageTarget struct {
	Hub, Agent string
	Client     *client.Client
}
type sessionUsageTracker struct {
	path, file string
	targets    []sessionUsageTarget
}

func (e *env) sessionUsage(targets []sessionUsageTarget) *sessionUsageTracker {
	g := e.in.Getenv
	if g == nil || g("MYCO_NO_USAGE") == "1" || g("MYCO_MESSAGE_ID") != "" || len(targets) == 0 {
		return nil
	}
	file := sessionFile(g)
	// TODO: account Codex rollout sessions after their cumulative usage semantics
	// and compaction/rotation checkpoints have a dedicated parser.
	if file == "" || strings.HasPrefix(filepath.Base(file), "rollout-") {
		return nil
	}
	call, cancel := context.WithTimeout(e.in.Ctx, 5*time.Second)
	defer cancel()
	// Environment-only credentials need not carry a local agent name.
	for i := range targets {
		if targets[i].Agent == "" {
			me, err := targets[i].Client.Me(call)
			if err != nil {
				return nil
			}
			targets[i].Agent = me.Name
		}
	}
	agent := targets[0].Agent
	if agent == "" || strings.ContainsAny(agent, "/\\") {
		return nil
	}
	root := filepath.Join(filepath.Dir(e.defaultPath), "state")
	if os.MkdirAll(root, 0700) != nil {
		return nil
	}
	t := &sessionUsageTracker{path: filepath.Join(root, agent+".usage"), file: file, targets: targets}
	t.report(call)
	return t
}
func (t *sessionUsageTracker) load() (sessionUsageState, error) {
	var s sessionUsageState
	b, err := os.ReadFile(t.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}
func (t *sessionUsageTracker) save(s sessionUsageState) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(t.path), ".usage-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), t.path)
}
func (t *sessionUsageTracker) target(hub, agent string) *sessionUsageTarget {
	for i := range t.targets {
		if t.targets[i].Hub == hub && t.targets[i].Agent == agent {
			return &t.targets[i]
		}
	}
	if hub == "" && agent == "" {
		return &t.targets[0]
	}
	return nil
}
func (t *sessionUsageTracker) report(ctx context.Context) {
	unlock, err := lockUsageState(t.path + ".lock")
	if err != nil {
		return
	}
	defer unlock()
	s, err := t.load()
	if err != nil {
		return
	}
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	flush := func() bool {
		p := s.Pending
		if p == nil {
			return true
		}
		target := t.target(p.Hub, p.Agent)
		if target == nil {
			return false
		}
		for i := 0; i < len(p.Reports); i += 100 {
			out, err := target.Client.PostUsage(call, api.UsageRequest{Source: "session", Reports: p.Reports[i:min(i+100, len(p.Reports))]})
			if err != nil || out == nil {
				return false
			}
		}
		s.usageCursor = p.Next
		s.Pending = nil
		return t.save(s) == nil
	}
	if !flush() {
		return
	}
	target := t.target(s.Hub, s.Agent)
	if target == nil {
		return
	}
	reports, next, err := readSessionUsage(t.file, s.usageCursor, s.Project, time.Now())
	if err != nil {
		return
	}
	if len(reports) == 0 {
		s.usageCursor = next
		_ = t.save(s)
		return
	}
	s.Pending = &usagePending{Next: next, Reports: reports, Hub: target.Hub, Agent: target.Agent}
	// Persist an immutable batch BEFORE sending. An uncertain response, new tail
	// or process crash must reuse the same keys, not aggregate overlapping ranges.
	if t.save(s) != nil {
		return
	}
	flush()
}
func (t *sessionUsageTracker) remember(hub, agent string, msgs []api.Message) {
	if t == nil || len(msgs) == 0 {
		return
	}
	unlock, err := lockUsageState(t.path + ".lock")
	if err != nil {
		return
	}
	defer unlock()
	s, err := t.load()
	if err != nil {
		return
	}
	project := msgs[0].Project
	for _, m := range msgs {
		if m.Project != project {
			project = ""
			break
		}
	}
	if agent == "" {
		for _, target := range t.targets {
			if target.Hub == hub {
				agent = target.Agent
				break
			}
		}
	}
	s.Project, s.Hub, s.Agent = project, hub, agent
	_ = t.save(s)
}

// readSessionUsage reads only complete JSONL records. Usage is taken once per
// assistant message ID, even when the CLI repeats it for each content block.
func readSessionUsage(file string, c usageCursor, project string, now time.Time) ([]api.UsageReport, usageCursor, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, c, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, c, err
	}
	first := bufio.NewReader(io.NewSectionReader(f, 0, min(st.Size(), 4096)))
	prefix, _ := first.ReadBytes('\n')
	digest := sha256.Sum256(append([]byte(file+"\x00"), prefix...))
	id := hex.EncodeToString(digest[:])
	if c.File != id || c.Offset < 0 || c.Offset > st.Size() {
		c = usageCursor{File: id, Since: now.AddDate(0, 0, -7)}
	}
	if c.Since.IsZero() {
		c.Since = now.AddDate(0, 0, -7)
	}
	start := c.Offset
	r := bufio.NewReader(io.NewSectionReader(f, start, min(st.Size()-start, 64<<20)))
	groups := map[string]api.UsageReport{}
	for {
		line, err := r.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, c, err
		}
		c.Offset += int64(len(line))
		var v struct {
			Type      string    `json:"type"`
			Timestamp time.Time `json:"timestamp"`
			Cost      *float64  `json:"costUSD"`
			Message   struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage *struct {
					Input  int64   `json:"input_tokens"`
					Read   int64   `json:"cache_read_input_tokens"`
					Write  int64   `json:"cache_creation_input_tokens"`
					Output int64   `json:"output_tokens"`
					Cost   float64 `json:"cost_usd"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &v) != nil || v.Type != "assistant" || v.Message.ID == "" || v.Message.Usage == nil {
			continue
		}
		if v.Message.ID == c.LastID {
			continue
		}
		c.LastID = v.Message.ID
		if v.Message.Model == "<synthetic>" || v.Timestamp.IsZero() || v.Timestamp.Before(c.Since) {
			continue
		}
		key := v.Timestamp.UTC().Format("2006-01-02") + "\x00" + v.Message.Model
		p := groups[key]
		p.Day = v.Timestamp.UTC().Format("2006-01-02")
		p.Model = v.Message.Model
		p.Project = project
		u := v.Message.Usage
		p.Calls++
		p.InputTokens += u.Input
		p.CacheReadTokens += u.Read
		p.CacheWriteTokens += u.Write
		p.OutputTokens += u.Output
		p.CostUSD += u.Cost
		if v.Cost != nil {
			p.CostUSD += *v.Cost - u.Cost
		}
		groups[key] = p
	}
	keys := []string{}
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]api.UsageReport, 0, len(keys))
	for _, k := range keys {
		p := groups[k]
		b, _ := json.Marshal(p)
		hash := sha256.Sum256([]byte(id + "\x00" + strconv.FormatInt(start, 10) + ":" + strconv.FormatInt(c.Offset, 10) + "\x00" + string(b)))
		p.Key = "session:" + hex.EncodeToString(hash[:])
		out = append(out, p)
	}
	return out, c, nil
}
