package im

import "fmt"

// What the bridge says, in the bridge's language ("zh" or "en").
var texts = map[string]map[string]string{
	"zh": {
		"help": "直接跟我说要做的事，我做完在这里回你。\n" +
			"- 我还在做的时候接着说：我会补上；我问你问题时，回复就是回答\n" +
			"- /stop：让我停下手上的事\n" +
			"- /new：下一条开一段新对话\n" +
			"- /whoami：你的用户 id",
		"no_agent":    "我还没连到 myco 里的 agent。",
		"not_allowed": "你还不能给我派活。请主人运行：myco im allow --name %s --user %s",
		"whoami":      "你的用户 id：%s",
		"new":         "好，下一条消息开一段新对话。",
		"stopped":     "⏹ 好，停下了。",
		"no_task":     "我现在手上没有在做的事。",
		"queued":      "⏳ 收到，我看看",
		"working":     "🔧 进行中：%s",
		"working0":    "🔧 进行中",
		"asking":      "❓ 我想确认一下：\n%s\n\n直接回复就行。",
		"done":        "✅ 好了",
		"failed":      "⚠️ 没做成：%s",
		"closed":      "⏹ 结束了（没有结果）",
		"steered":     "↪ 收到，补上了",
		"send_failed": "⚠️ 出了点问题：%s",
		"from":        "📨 %s 发来：\n%s",
		"via":         "（%s 上 %s 说的）",
		"via_words":   "（%s 上 %s 的原话：%s）",
		"rules":       "这个人不是主人。主人给这个机器人定的规矩，做的时候也要遵守：\n%s",
		"forwarded":   "已转到主人的%s私聊。",
		"unclaimed":   "我还没有主人。设置我的人请在私聊里发：/claim 码（码在 App 里这个 agent 的「聊天软件」那一行）。",
		"bad_claim":   "码不对。请用 App 里显示的认领码：/claim 码",
		"claim_gone":  "这个认领码已经失效（过期了，或者错了太多次）。请在 App 里这个 agent 的「聊天软件」那一行点「重新生成认领码」，或运行 myco im claim。",
		"claimed":     "好，你现在是我的主人。直接说要做的事，或发 /help 看用法。",
	},
	"en": {
		"help": "Tell me what to do; I answer here when it is done.\n" +
			"- While I am at it, what you say is added; when I ask, your reply answers\n" +
			"- /stop: stop what I am doing\n" +
			"- /new: start a new conversation with the next message\n" +
			"- /whoami: your user id",
		"no_agent":    "I am not connected to a myco agent yet.",
		"not_allowed": "You cannot give me work yet. Ask the owner to run: myco im allow --name %s --user %s",
		"whoami":      "Your user id: %s",
		"new":         "OK, your next message starts a new conversation.",
		"stopped":     "⏹ OK, stopped.",
		"no_task":     "I am not working on anything right now.",
		"queued":      "⏳ Got it, on it",
		"working":     "🔧 Working: %s",
		"working0":    "🔧 Working",
		"asking":      "❓ A quick question:\n%s\n\nJust reply here.",
		"done":        "✅ Done",
		"failed":      "⚠️ Could not do it: %s",
		"closed":      "⏹ Ended without a result",
		"steered":     "↪ Got it, added",
		"send_failed": "⚠️ Something went wrong: %s",
		"from":        "📨 From %s:\n%s",
		"via":         "(said by %[2]s on %[1]s)",
		"via_words":   "(%[2]s on %[1]s, in their words: %[3]s)",
		"rules":       "They are not the owner. The owner's rules for this bot, which the work keeps to as well:\n%s",
		"forwarded":   "Forwarded to the owner's %s direct messages.",
		"unclaimed":   "I have no owner yet. Whoever set me up: send /claim <code> here in a direct message (the code is on the agent's Chat apps row in the App).",
		"bad_claim":   "That code is not right. Use the claim code shown in the App: /claim <code>",
		"claim_gone":  "This claim code no longer works (it expired, or was mistyped too often). Make a new one: in the App, the agent's Chat apps row › New claim code, or myco im claim.",
		"claimed":     "Done: you own me now. Say what to do, or send /help.",
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
