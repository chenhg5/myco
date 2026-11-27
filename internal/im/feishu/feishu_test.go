package feishu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestRegistrationFlow(t *testing.T) {
	var polls atomic.Int32
	lark := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("action") == "poll" {
			w.Write([]byte(`{"client_id":"cli_x","client_secret":"sec","user_info":{"open_id":"ou_me","tenant_brand":"lark"}}`))
		}
	}))
	defer lark.Close()
	feishuSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("action") {
		case "init":
			w.Write([]byte(`{"supported_auth_methods":["client_secret"]}`))
		case "begin":
			if r.Form.Get("archetype") != "PersonalAgent" {
				t.Errorf("archetype %q", r.Form.Get("archetype"))
			}
			w.Write([]byte(`{"device_code":"dc","verification_uri_complete":"https://example/scan","interval":1,"expire_in":60}`))
		case "poll":
			if polls.Add(1) == 1 {
				w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			w.Write([]byte(`{"user_info":{"tenant_brand":"lark"}}`))
		}
	}))
	defer feishuSrv.Close()
	r := &Registrar{BaseURL: feishuSrv.URL, LarkURL: lark.URL}
	p, err := r.Begin(context.Background())
	if err != nil || p.URL != "https://example/scan" {
		t.Fatalf("begin %v %+v", err, p)
	}
	p.interval = 10 * time.Millisecond
	reg, err := p.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reg.AppID != "cli_x" || reg.AppSecret != "sec" || reg.OwnerOpenID != "ou_me" || reg.Platform != "lark" {
		t.Fatalf("registration %+v", reg)
	}
}

func TestRegistrationDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("action") {
		case "init":
			w.Write([]byte(`{}`))
		case "begin":
			w.Write([]byte(`{"device_code":"dc","verification_uri_complete":"u"}`))
		default:
			w.Write([]byte(`{"error":"access_denied"}`))
		}
	}))
	defer srv.Close()
	p, err := (&Registrar{BaseURL: srv.URL}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Wait(context.Background()); err == nil {
		t.Fatal("want an error")
	}
}

func sp(s string) *string { return &s }

func TestInbound(t *testing.T) {
	a := &Adapter{bot: "ou_bot"}
	ev := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender: &larkim.EventSender{SenderType: sp("user"), SenderId: &larkim.UserId{OpenId: sp("ou_me")}},
		Message: &larkim.EventMessage{MessageId: sp("om_2"), RootId: sp("om_1"), ChatId: sp("oc_g"), ChatType: sp("group"),
			MessageType: sp("text"), Content: sp(`{"text":"@_user_1 ask @_user_2 to review"}`),
			Mentions: []*larkim.MentionEvent{
				{Key: sp("@_user_1"), Id: &larkim.UserId{OpenId: sp("ou_bot")}, Name: sp("myco")},
				{Key: sp("@_user_2"), Id: &larkim.UserId{OpenId: sp("ou_ann")}, Name: sp("Ann")}}}}}
	in, ok := a.inbound(ev)
	if !ok || !in.Mentioned || !in.Group || in.Text != "ask @Ann to review" || in.ThreadKey != "msg:om_1" || in.UserID != "ou_me" {
		t.Fatalf("inbound %+v %v", in, ok)
	}
	ev.Event.Sender.SenderType = sp("bot")
	if _, ok := a.inbound(ev); ok {
		t.Fatal("a bot's message was taken")
	}
	ev.Event.Sender.SenderType = sp("user")
	ev.Event.Message.ChatType, ev.Event.Message.RootId, ev.Event.Message.Mentions = sp("p2p"), nil, nil
	ev.Event.Message.Content = sp(`{"text":"hi"}`)
	if in, _ := a.inbound(ev); in.Group || in.ThreadKey != "chat:oc_g" || in.Mentioned {
		t.Fatalf("direct %+v", in)
	}
}

func TestMessageText(t *testing.T) {
	post := `{"zh_cn":{"title":"Bug","content":[[{"tag":"text","text":"see "},{"tag":"a","text":"log","href":"https://x/log"}],[{"tag":"at","user_id":"@_user_1"},{"tag":"text","text":" fix"}]]}}`
	got, ok := messageText("post", post)
	if !ok || got != "Bug\nsee log (https://x/log)\n@_user_1 fix" {
		t.Fatalf("post %q", got)
	}
	if _, ok := messageText("image", `{}`); ok {
		t.Fatal("image taken as text")
	}
}

func TestContent(t *testing.T) {
	if k, _ := content("short"); k != larkim.MsgTypeInteractive {
		t.Fatal(k)
	}
	big := make([]byte, cardLimit+1)
	for i := range big {
		big[i] = 'a'
	}
	if k, _ := content(string(big)); k != larkim.MsgTypeText {
		t.Fatal(k)
	}
}

// Lark is its own app: its code is asked of Lark's accounts, not Feishu's.
func TestRegistrationLark(t *testing.T) {
	feishuSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Feishu asked: %s", r.URL)
	}))
	defer feishuSrv.Close()
	lark := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("action") {
		case "init":
			w.Write([]byte(`{"supported_auth_methods":["client_secret"]}`))
		case "begin":
			w.Write([]byte(`{"device_code":"dc","verification_uri_complete":"https://lark.example/scan","interval":1,"expires_in":60}`))
		case "poll":
			w.Write([]byte(`{"client_id":"cli_l","client_secret":"sec","user_info":{"open_id":"ou_l"}}`))
		}
	}))
	defer lark.Close()
	p, err := (&Registrar{BaseURL: feishuSrv.URL, LarkURL: lark.URL, Lark: true}).Begin(context.Background())
	if err != nil || p.URL != "https://lark.example/scan" {
		t.Fatalf("begin %v %+v", err, p)
	}
	p.interval = 10 * time.Millisecond
	reg, err := p.Wait(context.Background())
	if err != nil || reg.Platform != "lark" || reg.AppID != "cli_l" {
		t.Fatalf("registration %+v %v", reg, err)
	}
}
