// Package slack is the Slack side of a myco IM bridge: a bot that receives
// over Socket Mode (no public address needed).
package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/myco-ai/myco/internal/im"
)

// Adapter is a Slack bot.
type Adapter struct {
	api *slack.Client
	log *slog.Logger

	mu  sync.Mutex
	bot string // the bot's user id
}

// New makes an adapter from a bot token (xoxb-) and an app-level token
// (xapp-, with connections:write).
func New(botToken, appToken string, log *slog.Logger) *Adapter {
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{api: slack.New(botToken, slack.OptionAppLevelToken(appToken)), log: log}
}

// Check confirms the bot token works and returns the bot's name and team.
func (a *Adapter) Check(ctx context.Context) (string, error) {
	r, err := a.api.AuthTestContext(ctx)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.bot = r.UserID
	a.mu.Unlock()
	return r.User + " @ " + r.Team, nil
}

// Run connects over Socket Mode and passes messages to handle.
func (a *Adapter) Run(ctx context.Context, handle func(im.Inbound)) error {
	if _, err := a.Check(ctx); err != nil {
		return fmt.Errorf("slack: %w (check the bot token)", err)
	}
	sm := socketmode.New(a.api)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sm.Events:
				if !ok {
					return
				}
				a.event(sm, ev, handle)
			}
		}
	}()
	err := sm.RunContext(ctx)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (a *Adapter) event(sm *socketmode.Client, ev socketmode.Event, handle func(im.Inbound)) {
	switch ev.Type {
	case socketmode.EventTypeConnectionError:
		a.log.Warn("slack: connection error, reconnecting")
	case socketmode.EventTypeEventsAPI:
		if ev.Request != nil {
			sm.Ack(*ev.Request)
		}
		e, ok := ev.Data.(slackevents.EventsAPIEvent)
		if !ok || e.Type != slackevents.CallbackEvent {
			return
		}
		switch inner := e.InnerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			if inner.BotID != "" {
				return
			}
			if in, ok := a.inbound(inner.Channel, "channel", inner.User, inner.Text, inner.TimeStamp, inner.ThreadTimeStamp); ok {
				handle(in)
			}
		case *slackevents.MessageEvent:
			if inner.BotID != "" || inner.SubType != "" {
				return // bots (this one too), edits, joins
			}
			if inner.ChannelType != "im" && inner.ThreadTimeStamp == "" {
				return // top-level channel talk: only mentions (app_mention) are for the bot
			}
			if in, ok := a.inbound(inner.Channel, inner.ChannelType, inner.User, inner.Text, inner.TimeStamp, inner.ThreadTimeStamp); ok {
				handle(in)
			}
		}
	case socketmode.EventTypeSlashCommand:
		cmd, ok := ev.Data.(slack.SlashCommand)
		if !ok {
			return
		}
		if ev.Request != nil {
			sm.Ack(*ev.Request)
		}
		// "/myco agent coder" is "/agent coder" written to the bot.
		text := "/" + strings.TrimSpace(cmd.Text)
		if strings.TrimSpace(cmd.Text) == "" {
			text = "/help"
		}
		in := im.Inbound{ChatID: cmd.ChannelID, Group: !strings.HasPrefix(cmd.ChannelID, "D"),
			MessageID: "cmd:" + cmd.TriggerID, UserID: cmd.UserID, UserName: cmd.UserName, Text: text, Mentioned: true}
		in.ThreadKey = "chat:" + cmd.ChannelID
		handle(in)
	}
}

var mentionRE = regexp.MustCompile(`<@([A-Z0-9]+)(\|[^>]*)?>`)

func (a *Adapter) inbound(channel, channelType, user, text, ts, threadTS string) (im.Inbound, bool) {
	if channel == "" || user == "" || ts == "" {
		return im.Inbound{}, false
	}
	a.mu.Lock()
	bot := a.bot
	a.mu.Unlock()
	if user == bot {
		return im.Inbound{}, false
	}
	mentioned := false
	text = mentionRE.ReplaceAllStringFunc(text, func(m string) string {
		if id := mentionRE.FindStringSubmatch(m)[1]; id == bot {
			mentioned = true
			return ""
		}
		return m
	})
	in := im.Inbound{ChatID: channel, Group: channelType != "im", MessageID: channel + ":" + ts,
		UserID: user, Text: strings.TrimSpace(unescape(text)), Mentioned: mentioned}
	root := ts
	if threadTS != "" {
		root = threadTS
	}
	if in.Group {
		in.ThreadKey = "ts:" + channel + ":" + root
		in.ReplyTo = root
	} else {
		in.ThreadKey = "chat:" + channel
	}
	return in, true
}

func unescape(s string) string {
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(s)
}

// Slack messages hold up to 40,000 characters; sections 3,000.
const textLimit = 39000

func (a *Adapter) options(ref im.Ref, text string) []slack.MsgOption {
	opts := []slack.MsgOption{slack.MsgOptionText(clip(Mrkdwn(text), textLimit), false)}
	if ref.Thread && ref.ReplyTo != "" {
		opts = append(opts, slack.MsgOptionTS(ref.ReplyTo))
	}
	return opts
}

// Post sends text to the chat, in its thread for a group.
func (a *Adapter) Post(ctx context.Context, ref im.Ref, text string) (string, error) {
	_, ts, err := a.api.PostMessageContext(ctx, ref.ChatID, a.options(ref, text)...)
	return ts, err
}

// Status sends a message that Update can change.
func (a *Adapter) Status(ctx context.Context, ref im.Ref, text string) (string, error) {
	return a.Post(ctx, ref, text)
}

// Update changes a message the bot sent.
func (a *Adapter) Update(ctx context.Context, ref im.Ref, id, text string) error {
	_, _, _, err := a.api.UpdateMessageContext(ctx, ref.ChatID, id, slack.MsgOptionText(clip(Mrkdwn(text), textLimit), false))
	return err
}

// Direct sends text to a person in the app's direct messages.
func (a *Adapter) Direct(ctx context.Context, user, text string) error {
	_, _, err := a.api.PostMessageContext(ctx, user, slack.MsgOptionText(clip(Mrkdwn(text), textLimit), false))
	return err
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

var (
	boldRE    = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	linkRE    = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^)\s]+)\)`)
	headingRE = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
)

// Mrkdwn turns the Markdown agents write into Slack's mrkdwn (bold, links,
// headings; code blocks and lists read the same).
func Mrkdwn(s string) string {
	var out []string
	for i, part := range strings.Split(s, "```") {
		if i%2 == 1 { // inside a code block
			out = append(out, part)
			continue
		}
		part = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(part)
		part = headingRE.ReplaceAllString(part, "*$1*")
		part = boldRE.ReplaceAllString(part, "*$1*")
		part = linkRE.ReplaceAllStringFunc(part, func(m string) string {
			g := linkRE.FindStringSubmatch(m)
			return "<" + strings.ReplaceAll(g[2], "&amp;", "&") + "|" + g[1] + ">"
		})
		out = append(out, part)
	}
	return strings.Join(out, "```")
}

// Manifest is the app manifest a bridge needs: a bot over Socket Mode that
// hears mentions, direct messages and replies in threads, with /myco.
func Manifest(name string) map[string]any {
	return map[string]any{
		"_metadata":           map[string]any{"major_version": 2, "minor_version": 1},
		"display_information": map[string]any{"name": name, "description": "Talk to your myco agents", "background_color": "#2f6f4f"},
		"features": map[string]any{
			"app_home": map[string]any{"messages_tab_enabled": true, "messages_tab_read_only_enabled": false},
			"bot_user": map[string]any{"display_name": name, "always_online": true},
			"slash_commands": []map[string]any{{"command": "/myco", "description": "Talk to the myco bridge: help, agent, to, agents, stop, new",
				"usage_hint": "help | agent <name> | to <name> <task> | agents | stop | new", "should_escape": false}},
		},
		"oauth_config": map[string]any{"scopes": map[string]any{"bot": []string{
			"app_mentions:read", "chat:write", "commands", "im:history", "im:read", "im:write",
			"channels:history", "groups:history", "mpim:history", "users:read"}}},
		"settings": map[string]any{
			"event_subscriptions":    map[string]any{"bot_events": []string{"app_mention", "message.im", "message.channels", "message.groups", "message.mpim"}},
			"interactivity":          map[string]any{"is_enabled": true},
			"socket_mode_enabled":    true,
			"org_deploy_enabled":     false,
			"token_rotation_enabled": false,
		},
	}
}

// CreateURL opens Slack's "create an app" page with the manifest filled in.
func CreateURL(name string) string {
	b, _ := json.Marshal(Manifest(name))
	return "https://api.slack.com/apps?new_app=1&manifest_json=" + url.QueryEscape(string(b))
}
