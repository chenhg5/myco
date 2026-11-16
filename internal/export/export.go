// Package export turns a conversation's messages into a file people can
// keep or share: Markdown to read, and JSON Lines (myco's own fields) for
// programs. It is pure: the caller reads the messages (and, if wanted, the
// steps of each task) and writes the result.
//
// The formats are myco's own on purpose. Claude Code's and Codex's session
// files record one agent's model and tool calls; a myco conversation is
// messages between people and agents, so mapping one onto the other would
// lose or invent things.
package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
)

// Step is one thing an agent did for a task: a tool it ran and on what.
type Step struct {
	Tool   string `json:"tool"`
	Input  string `json:"input,omitempty"`
	Failed bool   `json:"failed,omitempty"`
}

// StepsOf picks the tool calls out of a task's trace, in order (what the
// agent said and thought, and the tools' output, are left out).
func StepsOf(events []api.TraceEvent) []Step {
	var out []Step
	at := map[string]int{} // a call's place, by its id, for its result
	for _, ev := range events {
		var e struct {
			Kind   string `json:"kind"`
			Tool   string `json:"tool"`
			ID     string `json:"id"`
			Input  string `json:"input"`
			Failed bool   `json:"failed"`
		}
		if json.Unmarshal(ev.Data, &e) != nil {
			continue
		}
		switch {
		case e.Kind == "tool" && e.Tool != "":
			if e.ID != "" {
				at[e.ID] = len(out)
			}
			out = append(out, Step{Tool: e.Tool, Input: oneLine(e.Input, 160)})
		case e.Kind == "tool_result" && e.Failed:
			if i, ok := at[e.ID]; ok {
				out[i].Failed = true
			}
		}
	}
	return out
}

// stopped is the hub's error on a task its sender stopped.
const stopped = "stopped by the sender"

var kindWords = map[string]string{
	"task": "任务", "reply": "回复", "progress": "进展", "steer": "补充", "note": "留言", "event": "事件",
}

// Markdown renders msgs (oldest first) as a document to read. steps, by
// task id, go under their task when given; loc is the time zone the times
// are written in (nil: local).
func Markdown(msgs []api.Message, steps map[string][]Step, now time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	var b strings.Builder
	b.WriteString("# 对话导出\n\n")
	fmt.Fprintf(&b, "导出时间：%s · 共 %d 条消息\n", now.In(loc).Format("2006-01-02 15:04"), len(msgs))
	for _, m := range msgs {
		b.WriteString("\n---\n\n")
		kind := kindWords[m.Type]
		if kind == "" {
			kind = m.Type
		}
		fmt.Fprintf(&b, "**%s** → %s · %s · %s", m.From, m.To, m.CreatedAt.In(loc).Format("01-02 15:04"), kind)
		switch {
		case m.Status == "failed" && m.Error == stopped:
			b.WriteString(" · 已停止")
		case m.Status == "failed" && m.Error != "":
			b.WriteString(" · 失败：" + oneLine(m.Error, 200))
		}
		b.WriteString("\n\n")
		if m.Unreadable != "" {
			b.WriteString("（无法解密：" + oneLine(m.Unreadable, 200) + "）\n")
		} else if body := strings.TrimRight(m.Body, "\n"); body != "" {
			b.WriteString(body + "\n")
		}
		if len(m.Attachments) > 0 {
			names := make([]string, 0, len(m.Attachments))
			for _, a := range m.Attachments {
				names = append(names, fmt.Sprintf("%s（%s）", a.Name, size(a.Size)))
			}
			b.WriteString("\n附件：" + strings.Join(names, "、") + "\n")
		}
		if ss := steps[m.ID]; len(ss) > 0 {
			fmt.Fprintf(&b, "\n<details><summary>过程：%d 步</summary>\n\n", len(ss))
			for _, s := range ss {
				line := s.Tool
				if s.Input != "" {
					line += " " + "`" + strings.ReplaceAll(s.Input, "`", "'") + "`"
				}
				if s.Failed {
					line += "（失败）"
				}
				b.WriteString("- " + line + "\n")
			}
			b.WriteString("\n</details>\n")
		}
	}
	return b.String()
}

// Line is one message as a JSON Lines record: myco's own field names,
// stable, with empty optional ones left out.
type Line struct {
	ID          string     `json:"id"`
	ThreadID    string     `json:"thread_id"`
	Project     string     `json:"project,omitempty"`
	From        string     `json:"from"`
	To          string     `json:"to"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	ReplyTo     string     `json:"reply_to,omitempty"`
	Body        string     `json:"body"`
	Unreadable  string     `json:"unreadable,omitempty"`
	Attachments []LineFile `json:"attachments"`
	CreatedAt   time.Time  `json:"created_at"`
	Steps       []Step     `json:"steps,omitempty"`
}

// LineFile is an attachment, by name (the file itself is not included).
type LineFile struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
}

// JSONL renders msgs as one JSON object per line (no header line).
func JSONL(msgs []api.Message, steps map[string][]Step) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, m := range msgs {
		l := Line{ID: m.ID, ThreadID: m.ThreadID, Project: m.Project, From: m.From, To: m.To, Type: m.Type,
			Status: m.Status, Error: m.Error, ReplyTo: m.ReplyTo, Body: m.Body, Unreadable: m.Unreadable,
			Attachments: []LineFile{}, CreatedAt: m.CreatedAt.UTC(), Steps: steps[m.ID]}
		if m.Unreadable != "" {
			l.Body = ""
		}
		for _, a := range m.Attachments {
			l.Attachments = append(l.Attachments, LineFile{Name: a.Name, Size: a.Size, ContentType: a.ContentType})
		}
		if err := enc.Encode(l); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

// size is a file size people read: B, KB or MB.
func size(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%d KB", (n+512)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// oneLine keeps s on one line, at most max runes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}
