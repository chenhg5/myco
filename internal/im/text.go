package im

import "fmt"

// What the bridge says, in the bridge's language ("zh" or "en").
var texts = map[string]map[string]string{
	"zh": {
		"help": "我会把你说的交给 myco 里的 agent，并在这里汇报进度和结果。\n" +
			"- 直接说要做的事：交给默认 agent（现在是 %s）\n" +
			"- 任务没完时接着说：补充给正在做的 agent；它提问时，回复就是回答\n" +
			"- /to 名字 要做的事：这一次交给别的 agent\n" +
			"- /agent 名字：换这个聊天的默认 agent\n" +
			"- /agents：在线的 agent\n" +
			"- /stop：停下正在做的任务\n" +
			"- /new：下一条开一段新对话\n" +
			"- /whoami：你的用户 id",
		"none":        "（未设置）",
		"no_agent":    "还没有默认 agent。用 /agent 名字 设一个，或 /to 名字 要做的事。",
		"not_allowed": "你还不能通过我给 agent 派活。请主人运行：myco im allow --name %s --user %s",
		"whoami":      "你的用户 id：%s",
		"agent_set":   "好，这个聊天之后交给 %s。",
		"new":         "好，下一条消息开一段新对话。",
		"stopped":     "⏹ 已让 %s 停下。",
		"no_task":     "现在没有在做的任务。",
		"queued":      "⏳ 已交给 %s",
		"working":     "🔧 %s 在做：%s",
		"working0":    "🔧 %s 在做",
		"asking":      "❓ %s 在问：\n%s\n\n在这里回复就是回答。",
		"done":        "✅ %s 完成",
		"failed":      "⚠️ %s 没做成：%s",
		"closed":      "⏹ 任务已结束（没有回复）",
		"steered":     "↪ 已补充给 %s",
		"send_failed": "⚠️ 没能交给 %s：%s",
		"agents":      "在线的 agent：\n%s",
		"no_agents":   "现在没有在线的 agent。",
		"from":        "📨 %s 发来：\n%s",
		"via":         "（经%s转述，来自 %s）",
		"forwarded":   "已转到主人的%s私聊。",
		"unclaimed":   "这个机器人还没有主人。设置它的人请在私聊里发：/claim 码（码在 myco im connect 的输出里）。",
		"bad_claim":   "码不对。请用 myco im connect 输出里的码：/claim 码",
		"claimed":     "好，你现在是这个机器人的主人。直接说要做的事，或发 /help 看用法。",
	},
	"en": {
		"help": "I pass what you say to agents in myco and report progress and results here.\n" +
			"- Say what to do: it goes to the default agent (now %s)\n" +
			"- While a task is open, what you say is added to it; when the agent asks, your reply answers\n" +
			"- /to name what to do: this once, to another agent\n" +
			"- /agent name: change this chat's default agent\n" +
			"- /agents: agents online\n" +
			"- /stop: stop the task at work\n" +
			"- /new: start a new conversation with the next message\n" +
			"- /whoami: your user id",
		"none":        "(not set)",
		"no_agent":    "No default agent yet. Set one with /agent name, or use /to name what to do.",
		"not_allowed": "You cannot give agents tasks through me yet. Ask the owner to run: myco im allow --name %s --user %s",
		"whoami":      "Your user id: %s",
		"agent_set":   "OK, this chat goes to %s from now on.",
		"new":         "OK, your next message starts a new conversation.",
		"stopped":     "⏹ Asked %s to stop.",
		"no_task":     "No task at work right now.",
		"queued":      "⏳ Sent to %s",
		"working":     "🔧 %s is working: %s",
		"working0":    "🔧 %s is working",
		"asking":      "❓ %s asks:\n%s\n\nReply here to answer.",
		"done":        "✅ %s is done",
		"failed":      "⚠️ %s could not do it: %s",
		"closed":      "⏹ The task ended without an answer",
		"steered":     "↪ Added for %s",
		"send_failed": "⚠️ Could not send it to %s: %s",
		"agents":      "Agents online:\n%s",
		"no_agents":   "No agents are online.",
		"from":        "📨 From %s:\n%s",
		"via":         "(relayed from %s, by %s)",
		"forwarded":   "Forwarded to the owner's %s direct messages.",
		"unclaimed":   "This bot has no owner yet. Whoever set it up: send /claim <code> here in a direct message (the code is in the output of myco im connect).",
		"bad_claim":   "That code is not right. Use the one from myco im connect: /claim <code>",
		"claimed":     "Done: you own this bot now. Say what to do, or send /help.",
	},
}

func (b *Bridge) t(key string, args ...any) string {
	m := texts[b.lang]
	if m == nil {
		m = texts["en"]
	}
	if len(args) == 0 {
		return m[key]
	}
	return fmt.Sprintf(m[key], args...)
}

// platformName is how people call the chat app, in the bridge's language.
func platformName(platform, lang string) string {
	switch platform {
	case "feishu":
		if lang == "zh" {
			return "飞书"
		}
		return "Feishu"
	case "lark":
		return "Lark"
	case "slack":
		return "Slack"
	}
	return platform
}

// DefaultLang is the language a bridge speaks unless set.
func DefaultLang(platform string) string {
	if platform == "feishu" {
		return "zh"
	}
	return "en"
}
