package export

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

var (
	loc = time.FixedZone("CST", 8*3600)
	t0  = time.Date(2026, 11, 16, 2, 3, 0, 0, time.UTC) // 10:03 in loc
)

func sample() []api.Message {
	return []api.Message{
		{ID: "m1", ThreadID: "m1", Project: "web", From: "@console:alice", To: "pm", Type: "task", Status: "done",
			Body: "把 README 的截图换成新版的。\n", CreatedAt: t0,
			Attachments: []api.Attachment{{FileID: "f1", Name: "a.png", Size: 12 * 1024, ContentType: "image/png"}, {FileID: "f2", Name: "b.log", Size: 300}}},
		{ID: "m2", ThreadID: "m1", From: "pm", To: "@console:alice", Type: "reply", ReplyTo: "m1", Status: "done",
			Body: "换好了。", CreatedAt: t0.Add(5 * time.Minute)},
		{ID: "m3", ThreadID: "m1", From: "@console:alice", To: "pm", Type: "task", Status: "failed",
			Error: "delivery attempts exhausted: recipient pulled but never acknowledged", Body: "再跑一遍测试", CreatedAt: t0.Add(6 * time.Minute)},
		{ID: "m4", ThreadID: "m1", From: "@console:alice", To: "pm", Type: "task", Status: "failed",
			Error: "stopped by the sender", Body: "算了", CreatedAt: t0.Add(7 * time.Minute)},
		{ID: "m5", ThreadID: "m1", From: "pm", To: "@console:alice", Type: "note", Status: "done",
			Unreadable: "no key for this device", Body: "e2e:envelope", CreatedAt: t0.Add(8 * time.Minute)},
	}
}

func TestStepsOf(t *testing.T) {
	ev := func(seq int, v any) api.TraceEvent {
		b, _ := json.Marshal(v)
		return api.TraceEvent{Seq: seq, Data: b}
	}
	got := StepsOf([]api.TraceEvent{
		ev(1, map[string]any{"kind": "text", "text": "Running the tests."}),
		ev(2, map[string]any{"kind": "tool", "tool": "Bash", "id": "t1", "input": "make\n  test"}),
		ev(3, map[string]any{"kind": "tool_result", "id": "t1", "failed": true, "output": "FAIL"}),
		ev(4, map[string]any{"kind": "tool", "tool": "Read", "id": "t2", "input": "/repo/x.go"}),
		ev(5, map[string]any{"kind": "tool_result", "id": "t2", "output": "ok"}),
		{Seq: 6, Data: json.RawMessage(`not json`)},
	})
	want := []Step{{Tool: "Bash", Input: "make test", Failed: true}, {Tool: "Read", Input: "/repo/x.go"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("steps: %+v", got)
	}
}

func TestMarkdown(t *testing.T) {
	steps := map[string][]Step{"m1": {{Tool: "Bash", Input: "cp `new.png` docs/", Failed: true}, {Tool: "Read", Input: "README.md"}}}
	got := Markdown(sample(), steps, t0.Add(time.Hour), loc)
	want := "# 对话导出\n\n" +
		"导出时间：2026-11-16 11:03 · 共 5 条消息\n" +
		"\n---\n\n" +
		"**@console:alice** → pm · 11-16 10:03 · 任务\n\n" +
		"把 README 的截图换成新版的。\n" +
		"\n附件：a.png（12 KB）、b.log（300 B）\n" +
		"\n<details><summary>过程：2 步</summary>\n\n" +
		"- Bash `cp 'new.png' docs/`（失败）\n" +
		"- Read `README.md`\n" +
		"\n</details>\n" +
		"\n---\n\n" +
		"**pm** → @console:alice · 11-16 10:08 · 回复\n\n" +
		"换好了。\n" +
		"\n---\n\n" +
		"**@console:alice** → pm · 11-16 10:09 · 任务 · 失败：delivery attempts exhausted: recipient pulled but never acknowledged\n\n" +
		"再跑一遍测试\n" +
		"\n---\n\n" +
		"**@console:alice** → pm · 11-16 10:10 · 任务 · 已停止\n\n" +
		"算了\n" +
		"\n---\n\n" +
		"**pm** → @console:alice · 11-16 10:11 · 留言\n\n" +
		"（无法解密：no key for this device）\n"
	if got != want {
		t.Fatalf("markdown:\n%s\n--- want ---\n%s", got, want)
	}
	if strings.Contains(got, "e2e:envelope") {
		t.Fatal("an unreadable body must not be written")
	}
}

func TestJSONL(t *testing.T) {
	steps := map[string][]Step{"m1": {{Tool: "Bash", Input: "make test"}}}
	out, err := JSONL(sample(), steps)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("%d lines:\n%s", len(lines), out)
	}
	want := `{"id":"m1","thread_id":"m1","project":"web","from":"@console:alice","to":"pm","type":"task","status":"done","body":"把 README 的截图换成新版的。\n","attachments":[{"name":"a.png","size":12288,"content_type":"image/png"},{"name":"b.log","size":300}],"created_at":"2026-11-16T02:03:00Z","steps":[{"tool":"Bash","input":"make test"}]}`
	if lines[0] != want {
		t.Fatalf("line 1:\n%s\nwant\n%s", lines[0], want)
	}
	var l2 map[string]any
	if json.Unmarshal([]byte(lines[1]), &l2) != nil || l2["reply_to"] != "m1" || l2["error"] != nil || l2["steps"] != nil || l2["project"] != nil {
		t.Fatalf("line 2: %s", lines[1])
	}
	if a, ok := l2["attachments"].([]any); !ok || len(a) != 0 {
		t.Fatalf("attachments must be a list, empty when none: %s", lines[1])
	}
	var l5 Line
	if json.Unmarshal([]byte(lines[4]), &l5) != nil || l5.Body != "" || l5.Unreadable != "no key for this device" {
		t.Fatalf("unreadable: %s", lines[4])
	}
}
