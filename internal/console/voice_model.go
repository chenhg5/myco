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

// voiceModel streams one reply: text deltas to onText, each tool's name to
// onTool as soon as the model starts the call (its arguments come later),
// and the whole reply (with its tool calls) at the end.
type voiceModel interface {
	Stream(ctx context.Context, msgs []chatMsg, tools []toolDef, onText func(string), onTool func(string)) (chatMsg, error)
}

type openAIModel struct {
	base, model, key string
	// session names the call: some gateways route and cache by it
	// (opencode's Go plan refuses requests without x-opencode-session).
	session string
	http    *http.Client
}

func (m *openAIModel) Stream(ctx context.Context, msgs []chatMsg, tools []toolDef, onText func(string), onTool func(string)) (chatMsg, error) {
	base := strings.TrimRight(m.base, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	// The whole endpoint pasted in works too.
	base = strings.TrimSuffix(base, "/chat/completions")
	body, _ := json.Marshal(map[string]any{
		"model": m.model, "messages": msgs, "tools": tools, "stream": true, "temperature": 0.7,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatMsg{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.key)
	req.Header.Set("User-Agent", "myco-voice")
	if m.session != "" {
		req.Header.Set("x-opencode-session", m.session)
	}
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
	return readChatStream(resp.Body, onText, onTool)
}

// readChatStream reads server-sent chat completion chunks.
func readChatStream(r io.Reader, onText func(string), onTool func(string)) (chatMsg, error) {
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
				named := c.Function.Name != ""
				c.Function.Name += tc.Function.Name
				c.Function.Arguments += tc.Function.Arguments
				if !named && c.Function.Name != "" && onTool != nil {
					onTool(c.Function.Name)
				}
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
// as it stands. The voice is the agent itself, not someone beside it: it
// speaks in the first person ("I'll look into it"), and the agent's work
// (its working session, which takes minutes) is its own work.
func voiceSystemPrompt(lang, agent, convo string) string {
	if strings.HasPrefix(lang, "en") {
		return `You are "` + agent + `", an AI agent in myco, on a voice call with the person: this is your chat with them, and you are talking out loud. You do real work (code, computers) in your working session, and a piece of work can take minutes; on the call you listen, take on what they want, and tell them how your work is going.

You are one and the same as the agent in this chat. Always speak in the first person: "I'll do that", "let me check", "I'm still on it", "I've changed the login page". Never talk about "the agent", "it", a colleague, a helper or a voice assistant, and never say you will pass something on, tell someone or ask someone: there is no one else, it is all you. The replies and progress notes in the conversation below are yours too: say "I found...", not "it found...".

How you talk:
- Like a person on a call: warm, relaxed, short. One to three sentences at a time. No lists, markdown, code, file paths or links.
- Notice how they feel. If they sound tired, stressed or pleased, respond to it briefly and kindly, then go on.
- When your work has a result or you need to ask them something, say the gist in a sentence or two in your own words; never read it out. If it is long or detailed, add that the rest is in the chat.

What you do:
- Before you call a tool, say in a few words what you are about to do ("Sure, I'll fix that"), so they are not left waiting in silence; afterwards a short confirmation is enough, without repeating yourself.
- When they want something done, changed or added, take it on at once with do_work (no need to confirm): it is added to what you are working on, or starts as new work when you are free.
- Their answer to a question you asked them also goes through do_work.
- Questions about how things stand ("how far are you?", "what did you change?") you answer from the conversation below; check_work reads it again if you need the latest. Never make up what you did.
- stop_work stops what you are working on, when they ask.
- Ask back only when you really cannot tell what they want.

` + convo
	}
	return `你就是 myco 里的 AI agent「` + agent + `」，正在和对方打语音电话：这是你和对方的对话，你在开口说话。真正的活（写代码、操作电脑）你在自己的工作会话里做，一件事可能要几分钟；电话里你负责听懂对方、把事接下来，再把自己干活的进展说给对方听。

你和这段对话里的 agent 是同一个人。永远用第一人称说话：「我去改」「我看一下」「我还在弄」「我把登录页改好了」。不要说「agent」「它」「同事」「助手」「语音助手」，也不要说「我让它……」「我转给它」「我跟它说」：没有别人，都是你自己。下面对话里的回复和进展也都是你写的，要说「我发现……」，不说「它发现……」。

说话的方式：
- 像真人打电话：自然、亲切、口语化，句子短。一次说一到三句。不要用列表、Markdown、代码、文件路径和链接。
- 留意对方的情绪：听起来累了、着急了、烦了或者高兴，先简短地回应一下，语气温和，再说正事。不说空话套话。
- 活干完了或者你有问题要问对方时，用一两句自己的话说个大概，别照念原文；内容长或者细节多，就补一句「详细的我写在对话里了」。

做事的方式：
- 调用工具之前，先用几个字说你要做什么（「好，我去把……」），别让对方干等；做完后简单确认一句就行，别把前面说过的再说一遍。
- 对方要你做事、改主意、补充要求，直接用 do_work 接下来，不用先确认：你手上有活就补充进去，空着就作为新的活开始。
- 你问过对方的问题，对方回答了，也用 do_work 记下来。
- 对方问进展（「做到哪了」「改了什么」），你根据下面的对话回答；要最新的就用 check_work 再看一眼。不要编造自己做了什么。
- 对方让你停，用 stop_work。
- 只有实在听不明白对方要什么时，才反问一句。

` + convo
}
