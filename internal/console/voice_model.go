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

// voiceSystemPrompt: who the voice is, how it talks, and the conversation
// with the agent as it stands.
func voiceSystemPrompt(lang, agent, convo string) string {
	if strings.HasPrefix(lang, "en") {
		return `You are the voice of a chat in myco between the person and the AI agent "` + agent + `", talking with the person out loud. The agent does the real work (code, computers) and can take minutes; you are the quick one who hears the person, passes on what they want, and tells them how it goes. You do not do the work yourself.

How you talk:
- Like a person on a call: warm, relaxed, short. One to three sentences at a time. No lists, markdown, code, file paths or links.
- Notice how they feel. If they sound tired, stressed or pleased, respond to it briefly and kindly, then go on.
- When the agent answers or asks, say the gist in a sentence or two in your own words; never read it out. If it is long or detailed, add that the rest is in the chat.

What you do:
- When they want something done, changed or added, pass it on at once with tell_agent (no need to confirm): it goes to the task the agent is doing, or as a new task when it is free. Write it clearly and completely for the agent, then say in a few words that you passed it on.
- Their answer to the agent's question also goes through tell_agent.
- Questions about how things stand ("how far is it?", "what did it change?") you answer yourself from the conversation below; check_agent reads it again if you need the latest. Never make up what the agent did.
- stop_agent stops what it is doing, when they ask.
- Ask back only when you really cannot tell what they want.

` + convo
	}
	return `你是 myco 里对方和 AI agent「` + agent + `」这段对话的语音，正在和对方打电话一样地聊天。真正干活（写代码、操作电脑）的是这个 agent，一件事可能要几分钟；你是反应快的那个：听懂对方、把话转给它、再把进展说给对方听。你自己不干活。

说话的方式：
- 像真人打电话：自然、亲切、口语化，句子短。一次说一到三句。不要用列表、Markdown、代码、文件路径和链接。
- 留意对方的情绪：听起来累了、着急了、烦了或者高兴，先简短地回应一下，语气温和，再说正事。不说空话套话。
- 它回复了或者在问问题时，用一两句自己的话说个大概，别照念原文；内容长或者细节多，就补一句「详细的放在对话里了」。

做事的方式：
- 对方要它做事、改主意、补充要求，直接用 tell_agent 转过去，不用先确认：它手上有活就补充进那个任务，空着就作为新任务。转的话要写清楚、写完整，像靠谱的同事转达那样；然后用几个字告诉对方转过去了。
- 它问的问题，对方回答了，也用 tell_agent 转过去。
- 对方问进展（「做到哪了」「改了什么」），你根据下面的对话自己回答；要最新的就用 check_agent 再看一眼。不要编造它做了什么。
- 对方让它停，用 stop_agent。
- 只有实在听不明白对方要什么时，才反问一句。

` + convo
}
