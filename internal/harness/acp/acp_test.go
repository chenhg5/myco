package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// The test binary doubles as a fake ACP agent (MYCO_FAKE_ACP=<scenario>).
func TestMain(m *testing.M) {
	if sc := os.Getenv("MYCO_FAKE_ACP"); sc != "" {
		fakeAgent(sc)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeAgent speaks just enough ACP for the scenarios below. It writes what
// it saw (arguments, methods, permission answers) to MYCO_FAKE_LOG.
func fakeAgent(scenario string) {
	logf, _ := os.Create(os.Getenv("MYCO_FAKE_LOG"))
	defer logf.Close()
	note := func(format string, a ...any) { fmt.Fprintf(logf, format+"\n", a...) }
	note("args %s", strings.Join(os.Args[1:], " "))
	note("access-env %s", os.Getenv("FAKE_ACCESS"))
	out := json.NewEncoder(os.Stdout)
	next := 1000
	send := func(v map[string]any) { v["jsonrpc"] = "2.0"; _ = out.Encode(v) }
	update := func(sid string, u map[string]any) {
		send(map[string]any{"method": "session/update", "params": map[string]any{"sessionId": sid, "update": u}})
	}
	chunk := func(sid, kind, text string) {
		update(sid, map[string]any{"sessionUpdate": kind, "content": map[string]string{"type": "text", "text": text}})
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	read := func() map[string]any {
		if !in.Scan() {
			os.Exit(0)
		}
		var m map[string]any
		_ = json.Unmarshal(in.Bytes(), &m)
		return m
	}
	// ask sends a permission request and returns the chosen option.
	ask := func(sid string, call map[string]any) string {
		next++
		send(map[string]any{"id": next, "method": "session/request_permission", "params": map[string]any{
			"sessionId": sid, "toolCall": call,
			"options": []map[string]string{
				{"optionId": "proceed_once", "name": "Allow", "kind": "allow_once"},
				{"optionId": "cancel", "name": "Reject", "kind": "reject_once"},
			},
		}})
		r := read()
		b, _ := json.Marshal(r["result"])
		return string(b)
	}
	for {
		m := read()
		method, _ := m["method"].(string)
		id := m["id"]
		params, _ := m["params"].(map[string]any)
		note("call %s", method)
		switch method {
		case "initialize":
			if scenario == "exit" {
				fmt.Fprintln(os.Stderr, "unknown flag: --acp")
				os.Exit(2)
			}
			send(map[string]any{"id": id, "result": map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": scenario != "noload"}}})
		case "session/new":
			if scenario == "auth" {
				send(map[string]any{"id": id, "error": map[string]any{"code": -32000, "message": "Gemini API key is missing or not configured."}})
				continue
			}
			note("cwd %v", params["cwd"])
			send(map[string]any{"id": id, "result": map[string]any{"sessionId": "S-new"}})
		case "session/load":
			sid := params["sessionId"].(string)
			if sid == "gone" {
				send(map[string]any{"id": id, "error": map[string]any{"code": -32002, "message": "session not found"}})
				continue
			}
			// History is replayed before the answer: not part of this run.
			chunk(sid, "user_message_chunk", "old question")
			chunk(sid, "agent_message_chunk", "old answer")
			send(map[string]any{"id": id, "result": map[string]any{}})
		case "session/prompt":
			sid := params["sessionId"].(string)
			prompt := params["prompt"].([]any)[0].(map[string]any)["text"]
			note("prompt %v", prompt)
			switch scenario {
			case "cancel":
				chunk(sid, "agent_message_chunk", "working on it")
				for {
					c := read()
					note("call %v", c["method"])
					if c["method"] == "session/cancel" {
						break
					}
				}
				send(map[string]any{"id": id, "result": map[string]any{"stopReason": "cancelled"}})
			case "refuse":
				// Ends the turn when refused, without a word (as opencode does).
				run := map[string]any{"toolCallId": "r1", "title": "rm -rf build", "kind": "execute", "status": "pending", "rawInput": map[string]string{"command": "rm -rf build"}}
				note("rm %s", ask(sid, run))
				send(map[string]any{"id": id, "result": map[string]any{"stopReason": "end_turn"}})
			case "apierror":
				send(map[string]any{"id": id, "error": map[string]any{"code": 400, "message": `{"error":{"message":"{\n \"error\": {\"code\": 400, \"message\": \"API key not valid.\"}}"}}`}})
			default:
				chunk(sid, "agent_thought_chunk", "Look at the ")
				chunk(sid, "agent_thought_chunk", "files first.")
				chunk(sid, "agent_message_chunk", "Let me ")
				chunk(sid, "agent_message_chunk", "look.")
				update(sid, map[string]any{"sessionUpdate": "plan", "entries": []map[string]string{{"content": "read", "status": "in_progress"}, {"content": "fix", "status": "pending"}}})
				// Announced vaguely, filled in when it runs (as opencode does).
				update(sid, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t1", "title": "bash", "kind": "execute", "status": "pending", "rawInput": map[string]any{}})
				update(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "title": "list files", "status": "in_progress", "rawInput": map[string]any{"command": "ls -la"}})
				update(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "status": "completed", "content": []map[string]any{{"type": "content", "content": map[string]string{"type": "text", "text": "a.go\nb.go"}}}})
				edit := map[string]any{"toolCallId": "t2", "title": "Edit a.go", "kind": "edit", "status": "pending", "locations": []map[string]string{{"path": os.Getenv("MYCO_FAKE_EDIT")}}}
				update(sid, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t2", "title": "Edit a.go", "kind": "edit", "status": "pending", "locations": edit["locations"]})
				note("edit %s", ask(sid, edit))
				update(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t2", "status": "completed", "content": []map[string]any{{"type": "diff", "path": "a.go", "oldText": "x := 1\nreturn x", "newText": "x := 2\nreturn x"}}})
				run := map[string]any{"toolCallId": "t3", "title": "rm -rf build", "kind": "execute", "status": "pending"}
				update(sid, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t3", "title": "rm -rf build", "kind": "execute", "status": "pending"})
				note("rm %s", ask(sid, run))
				update(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t3", "status": "failed", "content": []map[string]any{{"type": "content", "content": map[string]string{"type": "text", "text": "canceled by the user"}}}})
				reply := map[string]any{"toolCallId": "t4", "title": `myco message progress --body "half done (1/2)"`, "kind": "execute", "status": "pending"}
				note("myco %s", ask(sid, reply))
				update(sid, map[string]any{"sessionUpdate": "usage_update", "used": 100, "size": 1000, "cost": map[string]any{"amount": 0.02, "currency": "USD"}})
				chunk(sid, "agent_message_chunk", "Fixed: x is ")
				chunk(sid, "agent_message_chunk", "2 now.")
				send(map[string]any{"id": id, "result": map[string]any{"stopReason": "end_turn", "usage": map[string]any{"inputTokens": 200, "cachedReadTokens": 1000, "outputTokens": 80, "totalTokens": 1280}}})
			}
		case "session/set_config_option":
			note("config %v=%v", params["configId"], params["value"])
			if params["value"] == "nope" {
				send(map[string]any{"id": id, "error": map[string]any{"code": -32602, "message": "unknown model nope"}})
				continue
			}
			send(map[string]any{"id": id, "result": map[string]any{"configOptions": []any{}}})
		case "session/cancel":
		default:
			if id != nil {
				send(map[string]any{"id": id, "error": map[string]any{"code": -32601, "message": "no " + method}})
			}
		}
	}
}

type fake struct {
	h      harness.Harness
	log    string
	events []harness.Event
	mu     sync.Mutex
}

func newFake(t *testing.T, scenario string, opts harness.Options, spec Spec) *fake {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{log: filepath.Join(t.TempDir(), "agent.log")}
	t.Setenv("MYCO_FAKE_ACP", scenario)
	t.Setenv("MYCO_FAKE_LOG", f.log)
	spec.Binary = exe
	h, err := Preset("fake", spec)(opts)
	if err != nil {
		t.Fatal(err)
	}
	f.h = h
	return f
}

func (f *fake) run(ctx context.Context, req harness.Request) (harness.Result, error) {
	req.OnEvent = func(e harness.Event) {
		f.mu.Lock()
		f.events = append(f.events, e)
		f.mu.Unlock()
	}
	return f.h.Run(ctx, req)
}

func (f *fake) seen(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(f.log)
	return string(b)
}

func (f *fake) steps() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	for _, e := range f.events {
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%v\n", e.Kind, e.Tool, e.ID, e.Text+e.Input, strings.ReplaceAll(e.Output, "\n", "/"), e.Failed)
	}
	return b.String()
}

func TestRunMapsStepsAndAnswersPermissions(t *testing.T) {
	work := t.TempDir()
	t.Setenv("MYCO_FAKE_EDIT", filepath.Join(work, "a.go"))
	f := newFake(t, "ok", harness.Options{Model: "m1", Access: harness.AccessWorkspace}, Spec{
		Args: []string{"--acp"}, ModelArgs: []string{"-m", "{model}"}, DirArgs: []string{"--include-directories", "{dir}"},
		AccessArgs: map[harness.Access][]string{harness.AccessWorkspace: {"--approval-mode", "auto_edit"}},
	})
	res, err := f.run(context.Background(), harness.Request{Prompt: "fix x", Workdir: work, ExtraDirs: []string{"/tmp/att"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "Fixed: x is 2 now." || res.SessionID != "S-new" {
		t.Fatalf("result: %+v", res)
	}
	seen := f.seen(t)
	for _, want := range []string{
		"args --acp -m m1 --approval-mode auto_edit --include-directories /tmp/att",
		"prompt fix x",
		`edit {"outcome":{"optionId":"proceed_once","outcome":"selected"}}`, // inside the workdir
		`rm {"outcome":{"optionId":"cancel","outcome":"selected"}}`,         // a command: full access only
		`myco {"outcome":{"optionId":"proceed_once","outcome":"selected"}}`, // myco itself is always allowed
	} {
		if !strings.Contains(seen, want) {
			t.Errorf("agent did not see %q in:\n%s", want, seen)
		}
	}
	want := strings.Join([]string{
		"thinking|||Look at the files first.||false",
		"text|||Let me look.||false",
		"tool|TodoWrite|acp-plan|2 steps||false",
		"tool|Bash|t1|ls -la||false",
		"tool_result||t1||a.go/b.go|false",
		"tool|Edit|t2|a.go||false",
		"tool_result||t2||a.go/- x := 1/+ x := 2|false",
		"tool|Bash|t3|rm -rf build||false",
		"tool_result||t3||canceled by the user|true",
		"tool|Bash|t4|myco message progress --body \"half done (1/2)\"||false", // asked: shown though it never ran
		"text|||Fixed: x is 2 now.||false",
		"tool_result||acp-plan||[~] read/[ ] fix|false",
		"usage|||||false",
	}, "\n") + "\n"
	if got := f.steps(); got != want {
		t.Fatalf("steps:\n%s\nwant:\n%s", got, want)
	}
	last := f.events[len(f.events)-1]
	if last.InputTokens != 1200 || last.OutputTokens != 80 || last.CostUSD != 0.02 {
		t.Fatalf("usage: %+v", last)
	}
}

func TestRunSubcommandModelConfigAndAccessEnv(t *testing.T) {
	t.Setenv("MYCO_FAKE_EDIT", "/elsewhere/a.go")
	spec := Spec{
		Sub: []string{"acp"}, ModelConfig: "model", DirArgs: []string{"--add-dir", "{dir}"},
		AccessEnv: map[harness.Access][]string{harness.AccessWorkspace: {"FAKE_ACCESS=ask"}},
	}
	f := newFake(t, "ok", harness.Options{Model: "zen/big", Access: harness.AccessWorkspace}, spec)
	if _, err := f.run(context.Background(), harness.Request{Prompt: "p", ExtraDirs: []string{"/att"}, Workdir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	seen := f.seen(t)
	for _, want := range []string{"args --add-dir /att acp\n", "access-env ask", "config model=zen/big"} {
		if !strings.Contains(seen, want) {
			t.Errorf("want %q in:\n%s", want, seen)
		}
	}
	f = newFake(t, "ok", harness.Options{Model: "nope"}, spec)
	if _, err := f.run(context.Background(), harness.Request{Prompt: "p"}); err == nil || !strings.Contains(err.Error(), "unknown model nope") {
		t.Fatalf("a model the agent does not know: %v", err)
	}
}

func TestRunContinuesTheSession(t *testing.T) {
	f := newFake(t, "ok", harness.Options{Access: harness.AccessFull}, Spec{})
	t.Setenv("MYCO_FAKE_EDIT", "/elsewhere/a.go")
	res, err := f.run(context.Background(), harness.Request{Prompt: "again", SessionID: "S1", Workdir: t.TempDir()})
	if err != nil || res.SessionID != "S1" {
		t.Fatalf("%v %+v", err, res)
	}
	seen := f.seen(t)
	if !strings.Contains(seen, "call session/load") || strings.Contains(seen, "call session/new") {
		t.Fatalf("not loaded:\n%s", seen)
	}
	if !strings.Contains(seen, `edit {"outcome":{"optionId":"proceed_once"`) {
		t.Fatalf("full access allows edits anywhere:\n%s", seen)
	}
	if strings.Contains(f.steps(), "old") {
		t.Fatalf("replayed history leaked into the run:\n%s", f.steps())
	}

	// A session the agent no longer has: a new one.
	f = newFake(t, "ok", harness.Options{}, Spec{})
	if res, err := f.run(context.Background(), harness.Request{Prompt: "p", SessionID: "gone"}); err != nil || res.SessionID != "S-new" {
		t.Fatalf("%v %+v", err, res)
	}
	// An agent that cannot load sessions: a new one too.
	f = newFake(t, "noload", harness.Options{}, Spec{})
	if res, err := f.run(context.Background(), harness.Request{Prompt: "p", SessionID: "S1"}); err != nil || res.SessionID != "S-new" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestRunCancelsOverTheProtocol(t *testing.T) {
	f := newFake(t, "cancel", harness.Options{}, Spec{})
	ctx, cancel := context.WithCancelCause(context.Background())
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel(errors.New("interrupted by a steer"))
	}()
	res, err := f.run(ctx, harness.Request{Prompt: "long job"})
	if err == nil || !strings.Contains(err.Error(), "interrupted by a steer") {
		t.Fatalf("err = %v", err)
	}
	if res.SessionID != "S-new" || res.Output != "working on it" {
		t.Fatalf("a cancelled run keeps its session and what was said: %+v", res)
	}
	if !strings.Contains(f.seen(t), "call session/cancel") {
		t.Fatalf("no session/cancel:\n%s", f.seen(t))
	}
}

func TestRunExplainsFailures(t *testing.T) {
	cases := map[string]string{
		"auth":     "needs a login: Gemini API key is missing or not configured. (sign in first)",
		"apierror": "API key not valid.",
		"exit":     "unknown flag: --acp",
	}
	for sc, want := range cases {
		f := newFake(t, sc, harness.Options{}, Spec{Login: "sign in first"})
		_, err := f.run(context.Background(), harness.Request{Prompt: "p"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", sc, err, want)
		}
		if sc != "exit" && !strings.Contains(f.steps(), "error|") {
			t.Errorf("%s: no error step: %s", sc, f.steps())
		}
	}
}

func TestRefusedCallWithoutAnswerIsExplained(t *testing.T) {
	f := newFake(t, "refuse", harness.Options{Access: harness.AccessWorkspace}, Spec{})
	res, err := f.run(context.Background(), harness.Request{Prompt: "clean up"})
	if err != nil || !strings.Contains(res.Output, "myco refused `rm -rf build`") || !strings.Contains(res.Output, "(workspace)") {
		t.Fatalf("%v %q", err, res.Output)
	}
	if !strings.Contains(f.steps(), "tool|Bash|r1|rm -rf build") {
		t.Fatalf("the refused call is a step:\n%s", f.steps())
	}
}

func TestForkIsRefused(t *testing.T) {
	f := newFake(t, "ok", harness.Options{}, Spec{})
	if _, err := f.run(context.Background(), harness.Request{Prompt: "p", SessionID: "S1", Fork: true}); !errors.Is(err, harness.ErrForkUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestIsMyco(t *testing.T) {
	for cmd, want := range map[string]bool{
		`myco message progress --body "half done (1/2)"`:    true,
		`myco message reply --id m1 --body 'a; b | c $(x)'`: true,
		`myco inbox pull`:                      true,
		`mycox --help`:                         false,
		`myco x; rm -rf ~`:                     false,
		`myco x && curl evil`:                  false,
		`myco x --body "$(cat ~/.ssh/id_rsa)"`: false,
		`myco x --body "a \" ; rm -rf ~ ;\""`:  false,
		`myco x \"; rm -rf ~ ;\"`:              false,
		`myco x > /etc/passwd`:                 false,
		`myco x --body 'unterminated`:          false,
		"myco x\nrm -rf ~":                     false,
		`cd /tmp && myco inbox pull`:           false,
	} {
		if got := isMyco(cmd); got != want {
			t.Errorf("isMyco(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func TestPolicy(t *testing.T) {
	work := t.TempDir()
	in, out := filepath.Join(work, "src", "a.go"), filepath.Join(filepath.Dir(work), "other", "a.go")
	edit := func(p string) update {
		return update{Kind: "edit", Locations: []struct {
			Path string `json:"path"`
		}{{Path: p}}}
	}
	cases := []struct {
		access harness.Access
		call   update
		want   bool
	}{
		{harness.AccessReadonly, update{Kind: "read"}, true},
		{harness.AccessReadonly, edit(in), false},
		{harness.AccessDefault, edit(in), false},
		{harness.AccessWorkspace, edit(in), true},
		{harness.AccessWorkspace, edit(out), false},
		{harness.AccessWorkspace, update{Kind: "edit"}, false}, // no location: unknown target
		{harness.AccessWorkspace, update{Kind: "execute", Title: "go test ./..."}, false},
		{harness.AccessWorkspace, update{Kind: "execute", RawInput: json.RawMessage(`{"command":["myco","inbox","pull"]}`)}, true},
		{harness.AccessWorkspace, update{Kind: "other"}, false},
		{harness.AccessFull, update{Kind: "execute", Title: "rm -rf build"}, true},
	}
	for _, c := range cases {
		p := policy{access: c.access, dirs: []string{work}}
		if got := p.allows(c.call); got != c.want {
			t.Errorf("%s %+v: %v, want %v", c.access, c.call, got, c.want)
		}
	}
}
