package console

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// The memo of the last call: what was said in it is not in the chat (only
// what was passed on to the agent is), so a call ended loses it. When a
// call ends, its last few lines are kept on this computer, word for word,
// per workspace and agent; the next call with that agent within a day
// starts with them. No summary (no extra model call), nothing on the hub.

const (
	memoLines  = 8
	memoMaxAge = 24 * time.Hour
)

type voiceMemo struct {
	At    time.Time  `json:"at"`
	Lines []memoLine `json:"lines"`
}

type memoLine struct {
	// person | assistant
	Who  string `json:"who"`
	Text string `json:"text"`
}

func memoKey(hubID, agent string) string { return hubID + " " + agent }

// memoOf picks the last lines said in a call: the person's and the
// assistant's words, not the actions or the notices.
func memoOf(history []chatMsg) []memoLine {
	var out []memoLine
	for _, m := range history {
		text := strings.TrimSpace(m.Content)
		if text == "" {
			continue
		}
		switch m.Role {
		case "user":
			out = append(out, memoLine{Who: "person", Text: clipRunes(text, 300)})
		case "assistant":
			out = append(out, memoLine{Who: "assistant", Text: clipRunes(text, 300)})
		}
	}
	if len(out) > memoLines {
		out = out[len(out)-memoLines:]
	}
	return out
}

func (v *voice) memos() map[string]voiceMemo {
	all := map[string]voiceMemo{}
	if v.memoPath == "" {
		return all
	}
	if b, err := os.ReadFile(v.memoPath); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	return all
}

// keepMemo saves what was said in a call that ended; a call where nothing
// was said keeps the memo from before. Old memos are dropped.
func (v *voice) keepMemo(key string, history []chatMsg) {
	lines := memoOf(history)
	if v.memoPath == "" || len(lines) == 0 {
		return
	}
	v.memoMu.Lock()
	defer v.memoMu.Unlock()
	all := v.memos()
	for k, m := range all {
		if time.Since(m.At) > memoMaxAge {
			delete(all, k)
		}
	}
	all[key] = voiceMemo{At: time.Now(), Lines: lines}
	if b, err := json.MarshalIndent(all, "", "  "); err == nil {
		_ = os.WriteFile(v.memoPath, b, 0o600)
	}
}

// memo is the last call's memo with this agent, if it is recent.
func (v *voice) memo(key string) *voiceMemo {
	v.memoMu.Lock()
	defer v.memoMu.Unlock()
	m, ok := v.memos()[key]
	if !ok || len(m.Lines) == 0 || time.Since(m.At) > memoMaxAge {
		return nil
	}
	return &m
}

// memoPrompt is the memo as a part of the model's instructions.
func memoPrompt(lang string, m *voiceMemo) string {
	if m == nil {
		return ""
	}
	en := strings.HasPrefix(lang, "en")
	ago := time.Since(m.At)
	var when string
	switch {
	case ago < time.Hour:
		when = fmt.Sprintf("%d 分钟前", max(1, int(ago.Minutes())))
		if en {
			when = fmt.Sprintf("%d min ago", max(1, int(ago.Minutes())))
		}
	default:
		when = fmt.Sprintf("%d 小时前", int(ago.Hours()))
		if en {
			when = fmt.Sprintf("%d h ago", int(ago.Hours()))
		}
	}
	var b strings.Builder
	if en {
		b.WriteString("\nThe end of your last call with them (" + when + "), for reference; the chat above is what counts. Bring it up only if it matters now:\n")
	} else {
		b.WriteString("\n上一通电话（" + when + "）最后说的，供参考，以上面的对话为准；用得上再提，别主动复述：\n")
	}
	for _, l := range m.Lines {
		who := "对方"
		if en {
			who = "They"
		}
		if l.Who == "assistant" {
			who = "你"
			if en {
				who = "You"
			}
		}
		b.WriteString("- " + who + "：" + l.Text + "\n")
	}
	return b.String()
}
