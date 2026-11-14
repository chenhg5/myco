package console

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// What the voice assistant can do in myco: as the person, in the
// workspace the call was started in.
type voiceHub interface {
	Name() string
	// Members: who is in the workspace, a line each, for the model.
	Members(ctx context.Context) (string, error)
	Send(ctx context.Context, to, body string) (VoiceTaskRef, error)
	Steer(ctx context.Context, id, body string) error
	Stop(ctx context.Context, id string) error
	// Status says how a task stands, in a few lines for the model.
	Status(ctx context.Context, id string) (string, error)
	// TaskNews: whether a task has finished (and what it answered) or is
	// asking a question.
	TaskNews(ctx context.Context, id string) (taskNews, error)
}

type taskNews struct {
	Finished bool
	Summary  string // the answer (or why it failed), short
	Question string // the question it is waiting on
}

type hubActions struct {
	hubID, hubName string
	c              *client.Client
	// threads: the thread of each task handed out in this call (a console
	// reads tasks through their thread).
	mu      sync.Mutex
	threads map[string]string
}

// task finds a task handed out in this call and its thread.
func (h *hubActions) task(ctx context.Context, id string) (*api.Message, []api.Message, error) {
	h.mu.Lock()
	th := h.threads[id]
	h.mu.Unlock()
	if th == "" {
		return nil, nil, fmt.Errorf("task %s was not handed out in this call", id)
	}
	thread, err := h.c.Thread(ctx, th)
	if err != nil {
		return nil, nil, err
	}
	for i := range thread {
		if thread[i].ID == id {
			return &thread[i], thread, nil
		}
	}
	return nil, nil, fmt.Errorf("task %s not found", id)
}

func (h *hubActions) Name() string { return h.hubName }

func (h *hubActions) Members(ctx context.Context) (string, error) {
	ov, err := h.c.Overview(ctx, 0)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, s := range ov.Agents {
		a := s.Agent
		if a.Kind == "machine" || strings.HasSuffix(string(a.Name), "-machine") {
			continue
		}
		name := string(a.Name)
		line := name
		if a.DisplayName != "" {
			line += "（" + a.DisplayName + "）"
		}
		if a.Description != "" {
			line += "：" + clipRunes(a.Description, 80)
		}
		state := map[string]string{"working": "在干活", "pending": "有活在排队", "idle": "空闲", "offline": "离线"}[s.State]
		if state != "" {
			line += " [" + state + "]"
		}
		lines = append(lines, "- "+line)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

func (h *hubActions) Send(ctx context.Context, to, body string) (VoiceTaskRef, error) {
	res, err := h.c.ConsoleSend(ctx, api.SendRequest{To: to, Body: body})
	if err != nil {
		return VoiceTaskRef{}, err
	}
	m := res.Message
	h.mu.Lock()
	if h.threads == nil {
		h.threads = map[string]string{}
	}
	h.threads[m.ID] = m.ThreadID
	h.mu.Unlock()
	return VoiceTaskRef{HubID: h.hubID, ID: m.ID, Agent: m.To, Thread: m.ThreadID}, nil
}

func (h *hubActions) Steer(ctx context.Context, id, body string) error {
	_, err := h.c.ConsoleSteer(ctx, id, body)
	return err
}

func (h *hubActions) Stop(ctx context.Context, id string) error {
	_, err := h.c.ConsoleStop(ctx, id)
	return err
}

func (h *hubActions) Status(ctx context.Context, id string) (string, error) {
	m, thread, err := h.task(ctx, id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "任务 %s 给 %s，状态：%s\n", m.ID, m.To, statusWord(m.Status))
	var progress []string
	for _, x := range thread {
		if x.ReplyTo != m.ID {
			continue
		}
		switch x.Type {
		case "progress":
			progress = append(progress, clipRunes(x.Body, 200))
		case "reply":
			fmt.Fprintf(&b, "回复：%s\n", clipRunes(x.Body, 600))
		}
		if x.ID == m.Asking {
			fmt.Fprintf(&b, "它在问：%s\n", clipRunes(x.Body, 300))
		}
	}
	if n := len(progress); n > 0 {
		if n > 3 {
			progress = progress[n-3:]
		}
		fmt.Fprintf(&b, "最近的进展：\n- %s\n", strings.Join(progress, "\n- "))
	}
	if m.Error != "" {
		fmt.Fprintf(&b, "出错：%s\n", m.Error)
	}
	return b.String(), nil
}

func statusWord(s string) string {
	return map[string]string{"queued": "排队中", "delivered": "已送达，还没开始", "acked": "在做", "held": "先放着了", "done": "完成了", "failed": "失败或被停了"}[s] + "（" + s + "）"
}

func (h *hubActions) TaskNews(ctx context.Context, id string) (taskNews, error) {
	m, thread, err := h.task(ctx, id)
	if err != nil {
		return taskNews{}, err
	}
	var news taskNews
	for _, x := range thread {
		if x.ReplyTo == m.ID && x.Type == "reply" {
			news.Finished, news.Summary = true, clipRunes(x.Body, 600)
		}
		if m.Asking != "" && x.ID == m.Asking {
			news.Question = clipRunes(x.Body, 300)
		}
	}
	if !news.Finished && (m.Status == "done" || m.Status == "failed") {
		news.Finished = true
		news.Summary = "（结束了，没有回复）"
		if m.Error != "" {
			news.Summary = "没做成：" + m.Error
		}
	}
	return news, nil
}

// --- the tools the model gets ---

func tool(name, desc string, props map[string]any, required ...string) toolDef {
	var t toolDef
	t.Type = "function"
	t.Function.Name, t.Function.Description = name, desc
	t.Function.Parameters = map[string]any{"type": "object", "properties": props, "required": required}
	return t
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var voiceTools = []toolDef{
	tool("send_task", "Hand a task to an agent in the workspace, as the person (no need to confirm first).",
		map[string]any{
			"to":          str("The agent's name (its id, e.g. laptop-backend-claude), from the member list"),
			"task":        str("The task, written clearly and completely for the agent, in the person's language"),
			"what":        str("A few words naming the task, to tell the person about it later"),
			"their_words": str("What the person said, as heard"),
		}, "to", "task", "what"),
	tool("add_to_task", "Add something to a task already handed out (also how to pass on the person's answer to an agent's question).",
		map[string]any{"task_id": str("The task's id"), "text": str("What to add, written clearly for the agent")}, "task_id", "text"),
	tool("stop_task", "Stop a task.", map[string]any{"task_id": str("The task's id")}, "task_id"),
	tool("task_status", "How a task stands: status, latest progress, its answer or question.",
		map[string]any{"task_id": str("The task's id")}, "task_id"),
	tool("list_tasks", "The tasks handed out in this call, with their ids.", map[string]any{}),
	tool("list_members", "Who is in the workspace now, with what they do and whether they are busy.", map[string]any{}),
}

// runTool does what the model asked and says how it went (for the model).
func (c *voiceCall) runTool(ctx context.Context, call toolCall) string {
	var args map[string]string
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	act := func(text string, ref *VoiceTaskRef) {
		c.v.mu.Lock()
		c.v.add(VoiceEvent{Kind: vAction, Text: text, Task: ref})
		c.v.mu.Unlock()
	}
	switch call.Function.Name {
	case "send_task":
		to, task := strings.TrimSpace(args["to"]), strings.TrimSpace(args["task"])
		if to == "" || task == "" {
			return "error: to and task are required"
		}
		body := task + "\n\n——\n（经语音转述"
		if w := strings.TrimSpace(args["their_words"]); w != "" {
			body += "。原话：" + w
		}
		body += "）"
		ref, err := c.d.hub.Send(ctx, to, body)
		if err != nil {
			return "error: " + err.Error()
		}
		what := strings.TrimSpace(args["what"])
		if what == "" {
			what = clipRunes(task, 20)
		}
		act("交给 "+to+"："+what, &ref)
		c.watchTask(ref, what)
		return fmt.Sprintf("sent: task %s to %s", ref.ID, to)
	case "add_to_task":
		id, text := strings.TrimSpace(args["task_id"]), strings.TrimSpace(args["text"])
		if err := c.d.hub.Steer(ctx, id, text+"\n\n（经语音转述）"); err != nil {
			return "error: " + err.Error()
		}
		act("补充给任务 "+id+"："+clipRunes(text, 40), &VoiceTaskRef{HubID: c.hubID(), ID: id})
		return "added"
	case "stop_task":
		id := strings.TrimSpace(args["task_id"])
		if err := c.d.hub.Stop(ctx, id); err != nil {
			return "error: " + err.Error()
		}
		act("停下了任务 "+id, &VoiceTaskRef{HubID: c.hubID(), ID: id})
		return "stopped"
	case "task_status":
		s, err := c.d.hub.Status(ctx, strings.TrimSpace(args["task_id"]))
		if err != nil {
			return "error: " + err.Error()
		}
		return s
	case "list_tasks":
		done := make(chan string, 1)
		c.do(func() {
			var lines []string
			for _, w := range c.watched {
				state := "在做"
				if w.finished {
					state = "已结束"
				}
				lines = append(lines, fmt.Sprintf("- %s：%s → %s（%s）", w.ref.ID, w.what, w.ref.Agent, state))
			}
			sort.Strings(lines)
			if len(lines) == 0 {
				done <- "no tasks handed out in this call yet"
				return
			}
			done <- strings.Join(lines, "\n")
		})
		select {
		case s := <-done:
			return s
		case <-ctx.Done():
			return "cancelled"
		}
	case "list_members":
		m, err := c.d.hub.Members(ctx)
		if err != nil {
			return "error: " + err.Error()
		}
		return m
	}
	return "error: unknown tool " + call.Function.Name
}

func (c *voiceCall) hubID() string {
	if h, ok := c.d.hub.(*hubActions); ok {
		return h.hubID
	}
	return ""
}
