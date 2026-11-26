// Package im connects chat apps (Feishu/Lark, Slack) to myco agents.
//
// A bridge is one chat-app bot for one agent, speaking as one myco node
// (harness "inbox") owned by the person who set it up. What they write to the
// bot becomes a task for that agent, sent by the node; the agent's progress updates one status message
// in place, its answer comes back as a new message in the same conversation,
// and a question it asks is shown there for them to answer by replying.
//
// Conversations map to myco threads: a group topic (Feishu) or a thread
// (Slack) is one thread; a direct chat is one running thread until /new.
// While a task is open, what they write next is added to it (a steer, which
// also answers a waiting question); once it is closed, the next message is a
// new task in the same thread, so the agent's session keeps its context.
//
// Platform specifics (connections, cards, mentions) live in the adapters
// (internal/im/feishu, internal/im/slack); this package holds what is common.
package im

import (
	"context"
	"strings"
)

// Inbound is a message someone wrote to the bot.
type Inbound struct {
	ChatID    string
	Group     bool   // a group chat or channel (else a direct chat)
	MessageID string // the platform's id of this message
	// ThreadKey names the conversation it belongs to: the topic or thread
	// root in a group, the chat in a direct chat.
	ThreadKey string
	UserID    string
	UserName  string
	Text      string // without the mention of the bot
	Mentioned bool   // the bot was mentioned (groups)
	// ReplyTo is the message to answer in the thread of, when it is not
	// MessageID (Slack threads are named by their first message).
	ReplyTo string
}

// Ref is where to answer: a chat, and the message to answer in the thread of
// (empty: the chat itself).
type Ref struct {
	ChatID  string `json:"chat_id"`
	ReplyTo string `json:"reply_to,omitempty"`
	// Thread: answer in the message's thread (a group topic), not only
	// quoting it.
	Thread bool `json:"thread,omitempty"`
}

// Adapter is one chat app's side of a bridge.
type Adapter interface {
	// Run connects and calls handle for each message to the bot until ctx
	// ends. handle must not block for long.
	Run(ctx context.Context, handle func(Inbound)) error
	// Post sends text (Markdown) to ref and returns the new message's id.
	Post(ctx context.Context, ref Ref, text string) (string, error)
	// Status sends a status message that Update can change later.
	Status(ctx context.Context, ref Ref, text string) (string, error)
	// Update replaces a status message's text.
	Update(ctx context.Context, ref Ref, id, text string) error
	// Direct sends text to a person in a direct chat with the bot.
	Direct(ctx context.Context, userID, text string) error
}

// command is a command (/help, /to …) at the start of a message.
type command struct {
	name string // "", "help", "new", "stop", "whoami", "claim"
	arg  string // claim's code
	rest string // the text after it
}

func parseCommand(text string) command {
	t := strings.TrimSpace(text)
	// "!" works where the app takes "/" for its own commands (Slack).
	if !strings.HasPrefix(t, "/") && !strings.HasPrefix(t, "!") {
		return command{rest: t}
	}
	word, rest, _ := strings.Cut(t[1:], " ")
	rest = strings.TrimSpace(rest)
	switch w := strings.ToLower(word); w {
	case "help", "new", "stop", "whoami":
		return command{name: w, rest: rest}
	case "claim":
		arg, more, _ := strings.Cut(rest, " ")
		return command{name: w, arg: strings.TrimSpace(arg), rest: strings.TrimSpace(more)}
	}
	return command{rest: t} // not ours: a path, say
}

// clip shortens s to n runes, with an ellipsis.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
