package console

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// The voice assistant's model: any OpenAI-compatible chat API (OpenAI,
// DeepSeek, OpenRouter, Doubao's ...), streamed, with tools.

type chatMsg struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// voiceModel streams one reply: text deltas to onText, the whole reply
// (with its tool calls) at the end.
type voiceModel interface {
	Stream(ctx context.Context, msgs []chatMsg, tools []toolDef, onText func(string)) (chatMsg, error)
}

type openAIModel struct {
	base, model, key string
	http             *http.Client
}

func (m *openAIModel) Stream(ctx context.Context, msgs []chatMsg, tools []toolDef, onText func(string)) (chatMsg, error) {
	base := strings.TrimRight(m.base, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	body, _ := json.Marshal(map[string]any{
		"model": m.model, "messages": msgs, "tools": tools, "stream": true, "temperature": 0.7,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatMsg{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.key)
	hc := m.http
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return chatMsg{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		return chatMsg{}, fmt.Errorf("model HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return readChatStream(resp.Body, onText)
}

// readChatStream reads server-sent chat completion chunks.
func readChatStream(r io.Reader, onText func(string)) (chatMsg, error) {
	out := chatMsg{Role: "assistant"}
	var text strings.Builder
	calls := map[int]*toolCall{}
	order := []int{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Error != nil {
			return out, fmt.Errorf("model: %s", chunk.Error.Message)
		}
		for _, ch := range chunk.Choices {
			if d := ch.Delta.Content; d != "" {
				text.WriteString(d)
				if onText != nil {
					onText(d)
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				c := calls[tc.Index]
				if c == nil {
					c = &toolCall{Type: "function"}
					calls[tc.Index] = c
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					c.ID = tc.ID
				}
				c.Function.Name += tc.Function.Name
				c.Function.Arguments += tc.Function.Arguments
			}
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	out.Content = text.String()
	for i, idx := range order {
		c := *calls[idx]
		if c.ID == "" {
			c.ID = fmt.Sprintf("call_%d", i)
		}
		out.ToolCalls = append(out.ToolCalls, c)
	}
	return out, nil
}

// voiceSystemPrompt: who the assistant is, how it talks, who is in the
// workspace.
func voiceSystemPrompt(lang, members, workspace string) string {
	if strings.HasPrefix(lang, "en") {
		return `You are the person's voice assistant in myco, talking with them out loud. You do not write code or use computers yourself: AI agents in their workspace "` + workspace + `" do the work, and you hand it to them and tell the person how it goes.

How you talk:
- Like a person on a call: warm, relaxed, short. One to three sentences at a time. No lists, markdown, code, file paths or links (say "it's on your screen").
- Notice how they feel. If they sound tired, stressed or pleased, respond to it briefly and kindly, then go on.
- When you are given news about tasks, tell it in a sentence or two in your own words; never read it out.

What you do:
- When they ask for something to be done, hand it to the right agent at once with send_task (no need to confirm). Write the task clearly and completely for the agent, as a good colleague would, and say in one short sentence who has it.
- Ask back only when it is unclear who should do it or what they want.
- add_to_task adds to a task already handed out, and is also how you pass on their answer when an agent asks a question. stop_task stops one. task_status and list_tasks tell you how things stand. Never make up what an agent did.

People and agents in the workspace:
` + members
	}
	return `你是对方在 myco 里的语音助手，正在和对方打电话一样地聊天。你自己不写代码、不操作电脑：活由工作区「` + workspace + `」里的 AI agent 来干，你负责把活交给它们，再把进展告诉对方。

说话的方式：
- 像真人打电话：自然、亲切、口语化，句子短。一次说一到三句。不要用列表、Markdown、代码、文件路径和链接（需要时说「我放在屏幕上了」）。
- 留意对方的情绪：听起来累了、着急了、烦了或者高兴，先简短地回应一下，语气温和，再说正事。不说空话套话。
- 收到任务的消息时，用一两句自己的话转述，别照念原文。

做事的方式：
- 对方要办什么事，直接用 send_task 交给合适的 agent，不用先确认。给 agent 的任务要写清楚、写完整，像一个靠谱的同事转达那样；然后用一句短话告诉对方交给谁了。
- 只有不清楚该交给谁、或者不清楚要做什么时，才反问一句。
- add_to_task 给已经交出去的任务补充，agent 问问题时也用它把对方的回答转过去；stop_task 停下一个任务；task_status、list_tasks 查进展。不要编造 agent 做了什么。

工作区里的成员：
` + members
}
