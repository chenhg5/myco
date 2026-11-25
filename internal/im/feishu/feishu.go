// Package feishu is the Feishu/Lark side of a myco IM bridge: a bot that
// receives over Feishu's long connection (no public address needed) and
// answers with message cards.
package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/myco-ai/myco/internal/im"
)

// Adapter is a Feishu or Lark bot.
type Adapter struct {
	appID, appSecret string
	domain           string
	client           *lark.Client
	log              *slog.Logger

	mu  sync.Mutex
	bot string // the bot's own open_id
}

// New makes an adapter; platform is "feishu" or "lark".
func New(platform, appID, appSecret string, log *slog.Logger) *Adapter {
	domain := lark.FeishuBaseUrl
	if platform == "lark" {
		domain = lark.LarkBaseUrl
	}
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{appID: appID, appSecret: appSecret, domain: domain, log: log,
		client: lark.NewClient(appID, appSecret, lark.WithOpenBaseUrl(domain), lark.WithLogger(quiet{log}),
			lark.WithReqTimeout(20*time.Second))}
}

// Check confirms the credentials work and returns the bot's name.
func (a *Adapter) Check(ctx context.Context) (string, error) {
	id, name, err := a.botInfo(ctx)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.bot = id
	a.mu.Unlock()
	return name, nil
}

func (a *Adapter) botInfo(ctx context.Context) (openID, name string, err error) {
	resp, err := a.client.Get(ctx, "/open-apis/bot/v3/info", nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return "", "", err
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(resp.RawBody, &out); err != nil {
		return "", "", fmt.Errorf("bot info: %w", err)
	}
	if out.Code != 0 {
		return "", "", fmt.Errorf("bot info: code %d: %s", out.Code, out.Msg)
	}
	return out.Bot.OpenID, out.Bot.AppName, nil
}

// Run connects over the long connection and passes messages to handle.
func (a *Adapter) Run(ctx context.Context, handle func(im.Inbound)) error {
	if _, err := a.Check(ctx); err != nil {
		a.log.Warn("feishu: reading the bot's own id failed; group mentions are matched loosely", "err", err)
	}
	h := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(_ context.Context, ev *larkim.P2MessageReceiveV1) error {
			if in, ok := a.inbound(ev); ok {
				handle(in)
			}
			return nil
		}).
		OnP2MessageReadV1(func(context.Context, *larkim.P2MessageReadV1) error { return nil }).
		OnP2ChatAccessEventBotP2pChatEnteredV1(func(context.Context, *larkim.P2ChatAccessEventBotP2pChatEnteredV1) error { return nil })
	opts := []larkws.ClientOption{larkws.WithEventHandler(h), larkws.WithLogger(quiet{a.log}), larkws.WithLogLevel(larkcore.LogLevelWarn)}
	if a.domain != lark.FeishuBaseUrl {
		opts = append(opts, larkws.WithDomain(a.domain))
	}
	backoff := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := larkws.NewClient(a.appID, a.appSecret, opts...).Start(ctx)
		if ctx.Err() != nil {
			return nil
		}
		var ce *larkws.ClientError
		if errors.As(err, &ce) {
			return fmt.Errorf("feishu long connection: %w (check the app's credentials and that it is published)", err)
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		a.log.Warn("feishu: long connection ended, reconnecting", "err", err, "in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
	return nil
}

func (a *Adapter) inbound(ev *larkim.P2MessageReceiveV1) (im.Inbound, bool) {
	if ev == nil || ev.Event == nil || ev.Event.Message == nil || ev.Event.Sender == nil {
		return im.Inbound{}, false
	}
	m, s := ev.Event.Message, ev.Event.Sender
	if str(s.SenderType) != "user" || s.SenderId == nil {
		return im.Inbound{}, false // bots, including this one
	}
	text, ok := messageText(str(m.MessageType), str(m.Content))
	if !ok {
		return im.Inbound{}, false
	}
	a.mu.Lock()
	bot := a.bot
	a.mu.Unlock()
	mentioned := false
	for _, mt := range m.Mentions {
		if mt == nil || mt.Key == nil {
			continue
		}
		id := ""
		if mt.Id != nil {
			id = str(mt.Id.OpenId)
		}
		if bot == "" || id == bot {
			mentioned = true
			text = strings.ReplaceAll(text, *mt.Key, "")
		} else if mt.Name != nil {
			text = strings.ReplaceAll(text, *mt.Key, "@"+*mt.Name)
		}
	}
	in := im.Inbound{ChatID: str(m.ChatId), Group: str(m.ChatType) == "group", MessageID: str(m.MessageId),
		UserID: str(s.SenderId.OpenId), Text: strings.TrimSpace(text), Mentioned: mentioned}
	switch {
	case !in.Group:
		in.ThreadKey = "chat:" + in.ChatID
	case str(m.RootId) != "":
		in.ThreadKey = "msg:" + str(m.RootId)
	default:
		in.ThreadKey = "msg:" + in.MessageID
	}
	return in, in.ChatID != "" && in.UserID != ""
}

// messageText reads the text of a text or rich-text (post) message.
func messageText(kind, content string) (string, bool) {
	switch kind {
	case "text":
		var t struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content), &t) != nil {
			return "", false
		}
		return t.Text, true
	case "post":
		var raw map[string]json.RawMessage
		if json.Unmarshal([]byte(content), &raw) != nil {
			return "", false
		}
		body := []byte(content)
		if _, ok := raw["content"]; !ok { // wrapped in a locale: {"zh_cn": {...}}
			for _, v := range raw {
				body = v
				break
			}
		}
		var p struct {
			Title   string `json:"title"`
			Content [][]struct {
				Tag  string `json:"tag"`
				Text string `json:"text"`
				Href string `json:"href"`
				Key  string `json:"user_id"`
			} `json:"content"`
		}
		if json.Unmarshal(body, &p) != nil {
			return "", false
		}
		var lines []string
		if p.Title != "" {
			lines = append(lines, p.Title)
		}
		for _, para := range p.Content {
			var b strings.Builder
			for _, el := range para {
				switch el.Tag {
				case "text", "md":
					b.WriteString(el.Text)
				case "a":
					b.WriteString(el.Text)
					if el.Href != "" && el.Href != el.Text {
						b.WriteString(" (" + el.Href + ")")
					}
				case "at":
					b.WriteString(el.Key) // "@_user_1", replaced like a mention
				case "code_block":
					b.WriteString("\n```\n" + el.Text + "\n```\n")
				}
			}
			lines = append(lines, b.String())
		}
		return strings.Join(lines, "\n"), true
	}
	return "", false
}

// card is a message card with one Markdown block; it can be updated.
func card(text string) string {
	b, _ := json.Marshal(map[string]any{
		"schema": "2.0",
		"config": map[string]any{"update_multi": true},
		"body":   map[string]any{"elements": []map[string]any{{"tag": "markdown", "content": text}}},
	})
	return string(b)
}

// Cards hold about 30 KB; longer answers go as plain text.
const cardLimit = 24 << 10

func content(text string) (kind, body string) {
	if len(text) <= cardLimit {
		return larkim.MsgTypeInteractive, card(text)
	}
	b, _ := json.Marshal(map[string]string{"text": text})
	return larkim.MsgTypeText, string(b)
}

// Post sends text, as a card while it fits.
func (a *Adapter) Post(ctx context.Context, ref im.Ref, text string) (string, error) {
	kind, body := content(text)
	return a.send(ctx, ref, kind, body)
}

// Status sends a card that Update can change.
func (a *Adapter) Status(ctx context.Context, ref im.Ref, text string) (string, error) {
	return a.send(ctx, ref, larkim.MsgTypeInteractive, card(text))
}

// Update changes a card sent by Status.
func (a *Adapter) Update(ctx context.Context, _ im.Ref, id, text string) error {
	resp, err := a.client.Im.Message.Patch(ctx, larkim.NewPatchMessageReqBuilder().MessageId(id).
		Body(larkim.NewPatchMessageReqBodyBuilder().Content(card(clipBytes(text, cardLimit))).Build()).Build())
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("feishu: updating the card: code %d: %s", resp.Code, resp.Msg)
	}
	return nil
}

// Direct sends text to a person (by open_id) in their chat with the bot.
func (a *Adapter) Direct(ctx context.Context, openID, text string) error {
	kind, body := content(text)
	resp, err := a.client.Im.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().ReceiveIdType("open_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().ReceiveId(openID).MsgType(kind).Content(body).Build()).Build())
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("feishu: sending: code %d: %s", resp.Code, resp.Msg)
	}
	return nil
}

func (a *Adapter) send(ctx context.Context, ref im.Ref, kind, body string) (string, error) {
	if ref.ReplyTo != "" {
		resp, err := a.client.Im.Message.Reply(ctx, larkim.NewReplyMessageReqBuilder().MessageId(ref.ReplyTo).
			Body(larkim.NewReplyMessageReqBodyBuilder().MsgType(kind).Content(body).ReplyInThread(ref.Thread).Build()).Build())
		if err != nil {
			return "", err
		}
		if resp.Success() && resp.Data != nil {
			return str(resp.Data.MessageId), nil
		}
		// The message it answered may be gone (recalled): send to the chat.
		a.log.Debug("feishu: reply failed, sending to the chat", "code", resp.Code, "msg", resp.Msg)
	}
	resp, err := a.client.Im.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().ReceiveId(ref.ChatID).MsgType(kind).Content(body).Build()).Build())
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil {
		return "", fmt.Errorf("feishu: sending: code %d: %s", resp.Code, resp.Msg)
	}
	return str(resp.Data.MessageId), nil
}

func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s[:n])
	return string(r[:max(len(r)-2, 0)]) + "…"
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// quiet routes the SDK's log lines to slog (it logs a lot at info).
type quiet struct{ l *slog.Logger }

func (q quiet) Debug(_ context.Context, a ...any) { q.l.Debug(fmt.Sprint(a...)) }
func (q quiet) Info(_ context.Context, a ...any)  { q.l.Debug(fmt.Sprint(a...)) }
func (q quiet) Warn(_ context.Context, a ...any)  { q.l.Warn(fmt.Sprint(a...)) }
func (q quiet) Error(_ context.Context, a ...any) { q.l.Error(fmt.Sprint(a...)) }
