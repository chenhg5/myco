package console

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// A voice call is with one agent, in the person's conversation with it (as
// ChatGPT's and Claude's voice are in one chat). The voice is that agent:
// a fast model speaks for it, in the first person, while its working
// session does the work. What the person wants done goes to that session
// as if typed: a new task when it is free, an addition to its task at work
// otherwise (the App's rule for Enter). The fast model answers the rest
// itself ("how far are you?") from the conversation.
type voiceHub interface {
	// Name: what the person calls the agent.
	Name() string
	// Context: who the agent is, the conversation's last messages and the
	// task at work, for the model (read before each answer).
	Context(ctx context.Context) (string, error)
	// Tell passes what the person wants to the agent: added to its task at
	// work (steered), else a new task.
	Tell(ctx context.Context, body string) (ref VoiceTaskRef, steered bool, err error)
	// StopCurrent stops the task at work.
	StopCurrent(ctx context.Context) (VoiceTaskRef, error)
	// Current is the task at work, if any (watched from the start).
	Current(ctx context.Context) (*VoiceTaskRef, error)
	// TaskNews: whether a task has finished (and what it answered) or is
	// asking a question.
	TaskNews(ctx context.Context, ref VoiceTaskRef) (taskNews, error)
}

type taskNews struct {
	Finished bool
	Summary  string // the answer (or why it failed), short
	Question string // the question it is waiting on
	Status   string // the task's status (acked: at work)
	Since    time.Time
	Progress string // its newest progress note, short
}

// hubActions is a call's myco side: the person (peer, their console
// address) and one agent, in one workspace.
type hubActions struct {
	hubID       string
	agent, peer string
	c           *client.Client
	// en: what the model reads is in English (else Chinese).
	en bool

	mu      sync.Mutex
	profile *api.Agent
}

func (h *hubActions) Name() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.profile != nil && h.profile.DisplayName != "" {
		return h.profile.DisplayName
	}
	return h.agent
}

// conversation: the last messages between the person and the agent,
// oldest first.
func (h *hubActions) conversation(ctx context.Context) ([]api.Message, error) {
	ex, err := h.c.Exchanges(ctx, h.agent, h.peer, 0, 40)
	if err != nil {
		return nil, err
	}
	ms := ex.Messages
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].CreatedAt.Before(ms[j].CreatedAt) })
	return ms, nil
}

// currentTask is the person's newest task to the agent still open: what
// an addition goes to. One set aside (held) takes more only while it asks
// the person something, as in the App.
func currentTask(ms []api.Message, agent, peer string) *api.Message {
	answered := map[string]bool{}
	for _, m := range ms {
		if m.ReplyTo != "" && (m.Type == "reply" || m.Type == "event") {
			answered[m.ReplyTo] = true
		}
	}
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		if m.Type != "task" || m.From != peer || m.To != agent || answered[m.ID] {
			continue
		}
		if m.Status == "done" || m.Status == "failed" || (m.Status == "held" && m.Asking == "") {
			continue
		}
		return &m
	}
	return nil
}

func (h *hubActions) ref(m *api.Message) VoiceTaskRef {
	return VoiceTaskRef{HubID: h.hubID, ID: m.ID, Agent: h.agent, Thread: m.ThreadID}
}

func (h *hubActions) loadProfile(ctx context.Context) (*api.Agent, string) {
	ov, err := h.c.Overview(ctx, 0)
	if err != nil {
		return nil, ""
	}
	for _, s := range ov.Agents {
		if s.Agent.Name == h.agent {
			a := s.Agent
			h.mu.Lock()
			h.profile = &a
			h.mu.Unlock()
			return &a, s.State
		}
	}
	return nil, ""
}

var kindWords = map[string]string{"task": "任务", "reply": "回复", "progress": "进展", "steer": "补充", "event": "通知", "note": "留言"}

// Context is written for the model as the agent itself: "you" are the
// agent, "they" are the person on the call.
func (h *hubActions) Context(ctx context.Context) (string, error) {
	var b strings.Builder
	L := func(zh, en string) string {
		if h.en {
			return en
		}
		return zh
	}
	p, state := h.loadProfile(ctx)
	b.WriteString(L("你是：", "You are: ") + h.agent)
	if p != nil {
		if p.DisplayName != "" {
			b.WriteString(L("（"+p.DisplayName+"）", " ("+p.DisplayName+")"))
		}
		if p.Description != "" {
			b.WriteString(L("，", ", ") + clipRunes(p.Description, 200))
		}
	}
	states := map[string]string{"working": "你在干活", "pending": "你有活在排队", "idle": "你空着", "offline": "你的工作会话离线了"}
	if h.en {
		states = map[string]string{"working": "you are at work", "pending": "you have work queued", "idle": "you are free", "offline": "your working session is offline"}
	}
	if w := states[state]; w != "" {
		b.WriteString(L("。现在"+w, ". Right now "+w))
	}
	b.WriteString("\n")
	ms, err := h.conversation(ctx)
	if err != nil {
		return b.String(), err
	}
	if len(ms) == 0 {
		b.WriteString(L("\n你们还没有说过话。\n", "\nYou have not talked yet.\n"))
		return b.String(), nil
	}
	b.WriteString(L("\n对话里最近的消息（旧的在前；「你」是你自己，「对方」是正在和你通话的这个人）：\n",
		"\nThe latest messages in your chat (oldest first; \"You\" is you, \"They\" is the person on the call):\n"))
	shown := ms
	if len(shown) > 14 {
		shown = shown[len(shown)-14:]
	}
	for _, m := range shown {
		who := L("对方", "They")
		if m.From == h.agent {
			who = L("你", "You")
		}
		kind := m.Type
		if !h.en && kindWords[m.Type] != "" {
			kind = kindWords[m.Type]
		}
		fmt.Fprintf(&b, L("- %s %s（%s）：%s\n", "- %s %s (%s): %s\n"), m.CreatedAt.Local().Format("15:04"), who, kind, clipRunes(strings.TrimSpace(m.Body), 240))
	}
	if cur := currentTask(ms, h.agent, h.peer); cur != nil {
		fmt.Fprintf(&b, L("\n你手上正在做对方交的：%s（%s）\n", "\nYou are working on what they asked: %s (%s)\n"), clipRunes(cur.Body, 120), statusWord(h.en, cur.Status))
		if cur.Asking != "" {
			for _, m := range ms {
				if m.ID == cur.Asking {
					fmt.Fprintf(&b, L("你问了对方，在等回答：%s\n", "You asked them and are waiting for the answer: %s\n"), clipRunes(m.Body, 300))
				}
			}
		}
	} else {
		b.WriteString(L("\n你手上没有对方交的活。\n", "\nYou have nothing of theirs at work.\n"))
	}
	return b.String(), nil
}

func (h *hubActions) Current(ctx context.Context) (*VoiceTaskRef, error) {
	ms, err := h.conversation(ctx)
	if err != nil {
		return nil, err
	}
	if cur := currentTask(ms, h.agent, h.peer); cur != nil {
		r := h.ref(cur)
		return &r, nil
	}
	return nil, nil
}

func (h *hubActions) Tell(ctx context.Context, body string) (VoiceTaskRef, bool, error) {
	ms, err := h.conversation(ctx)
	if err != nil {
		return VoiceTaskRef{}, false, err
	}
	if cur := currentTask(ms, h.agent, h.peer); cur != nil {
		_, err := h.c.ConsoleSteer(ctx, cur.ID, body)
		if err == nil {
			return h.ref(cur), true, nil
		}
		if ce, ok := err.(*client.Error); !ok || ce.Code != "invalid_state" {
			return VoiceTaskRef{}, false, err
		}
		// It ended a moment ago: a new task, then.
	}
	res, err := h.c.ConsoleSend(ctx, api.SendRequest{To: h.agent, Body: body})
	if err != nil {
		return VoiceTaskRef{}, false, err
	}
	return h.ref(&res.Message), false, nil
}

func (h *hubActions) StopCurrent(ctx context.Context) (VoiceTaskRef, error) {
	ms, err := h.conversation(ctx)
	if err != nil {
		return VoiceTaskRef{}, err
	}
	cur := currentTask(ms, h.agent, h.peer)
	if cur == nil {
		return VoiceTaskRef{}, fmt.Errorf("nothing of theirs is at work")
	}
	if _, err := h.c.ConsoleStop(ctx, cur.ID); err != nil {
		return VoiceTaskRef{}, err
	}
	return h.ref(cur), nil
}

// statusWord: a task's status as the agent would say it of its own work.
func statusWord(en bool, s string) string {
	words := map[string]string{"queued": "排队中", "delivered": "收到了，还没开始", "acked": "在做", "held": "先放着了", "done": "做完了", "failed": "没做成或被停了"}
	if en {
		words = map[string]string{"queued": "queued", "delivered": "received, not started", "acked": "in progress", "held": "set aside", "done": "done", "failed": "failed or stopped"}
	}
	if w := words[s]; w != "" {
		return w
	}
	return s
}

// TaskNews reads the task through its thread (a console cannot read a
// message by id).
func (h *hubActions) TaskNews(ctx context.Context, ref VoiceTaskRef) (taskNews, error) {
	thread, err := h.c.Thread(ctx, ref.Thread)
	if err != nil {
		return taskNews{}, err
	}
	var m *api.Message
	for i := range thread {
		if thread[i].ID == ref.ID {
			m = &thread[i]
		}
	}
	if m == nil {
		return taskNews{}, fmt.Errorf("task %s not found", ref.ID)
	}
	news := taskNews{Status: m.Status, Since: m.CreatedAt}
	for _, x := range thread {
		if x.ReplyTo == m.ID && x.Type == "reply" {
			news.Finished, news.Summary = true, clipRunes(x.Body, 800)
		}
		if x.ReplyTo == m.ID && x.Type == "progress" && !x.Ask && strings.TrimSpace(x.Body) != "" {
			news.Progress = clipRunes(strings.TrimSpace(x.Body), 200) // the thread is oldest first
		}
		if m.Asking != "" && x.ID == m.Asking {
			news.Question = clipRunes(x.Body, 400)
		}
	}
	if !news.Finished && (m.Status == "done" || m.Status == "failed") {
		news.Finished = true
		news.Summary = "（结束了，没写回复）"
		if h.en {
			news.Summary = "(ended without a reply)"
		}
		if m.Error != "" {
			news.Summary = "没做成：" + m.Error
			if h.en {
				news.Summary = "did not work out: " + m.Error
			}
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

// The tools are named and described as the agent's own work, so the model
// speaks of it as its own ("I'll fix it"), not as someone else's.
var voiceTools = []toolDef{
	tool("do_work", "Take on what the person wants done, changed or added, or record their answer to a question you asked them (no need to confirm first). It is added to what you are working on if you have something at work, else it starts as new work.",
		map[string]any{
			"text":        str("The request as the person would type it in the chat, clear and complete, in their language (it goes into the chat as their message)"),
			"what":        str("A few words naming it, to tell the person about it later"),
			"their_words": str("What the person said, as heard"),
		}, "text", "what"),
	tool("stop_work", "Stop what you are working on.", map[string]any{}),
	tool("check_work", "Read your chat again now: your latest progress, result or question.", map[string]any{}),
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
	en := strings.HasPrefix(c.d.lang, "en")
	L := func(zh, en2 string) string {
		if en {
			return en2
		}
		return zh
	}
	switch call.Function.Name {
	case "do_work":
		text := strings.TrimSpace(args["text"])
		if text == "" {
			return "error: text is required"
		}
		body := text + "\n\n——\n（经语音转述"
		if w := strings.TrimSpace(args["their_words"]); w != "" {
			body += "。原话：" + w
		}
		body += "）"
		ref, steered, err := c.d.hub.Tell(ctx, body)
		if err != nil {
			return "error: " + err.Error()
		}
		what := strings.TrimSpace(args["what"])
		if what == "" {
			what = clipRunes(text, 20)
		}
		c.watchTask(ref, what)
		if steered {
			act(L("加到手上的活里："+what, "Added to what I'm doing: "+what), &ref)
			return "added to what you are working on"
		}
		act(L("开始做："+what, "On it: "+what), &ref)
		return "started: you are working on it now"
	case "stop_work":
		ref, err := c.d.hub.StopCurrent(ctx)
		if err != nil {
			return "error: " + err.Error()
		}
		act(L("停下了手上的活", "Stopped what I was doing"), &ref)
		return "stopped"
	case "check_work":
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		s, err := c.d.hub.Context(cctx)
		if err != nil {
			return "error: " + err.Error()
		}
		return s
	}
	return "error: unknown tool " + call.Function.Name
}
