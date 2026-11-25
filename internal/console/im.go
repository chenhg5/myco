package console

// An agent's chat-app bots (myco im) from the App: each agent can have its
// own Feishu/Lark bot (a QR code shown in the App, scanned with the phone)
// and Slack bot (an app made from a manifest, and its two tokens). The App
// shows them in the agent's details, and can remove them.
//
// The QR code is Feishu's app registration flow, run here so the App can
// show the code; the rest is the myco CLI (myco im connect/list/set/remove),
// which creates the bot's node on the hub and keeps the bot's secrets in
// ~/.myco/im. This computer's runner service runs the bots configured there.
//
//	GET    /app/im?hub_id=&agent=     {bridges: [...], slack_create_url}  (that agent's bots)
//	POST   /app/im/feishu             {hub_id, agent}  → {id, url, qr}  a QR code to scan
//	GET    /app/im/feishu/{id}        {state: waiting|connecting|done|failed, error?, bridge?}
//	DELETE /app/im/feishu/{id}        stop waiting for the scan
//	POST   /app/im/slack              {hub_id, agent, bot_token, app_token} → {bridge}
//	DELETE /app/im/{name}

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"rsc.io/qr"

	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/im"
	"github.com/myco-ai/myco/internal/im/feishu"
	imslack "github.com/myco-ai/myco/internal/im/slack"
)

func (b *Backend) imRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/im", b.getIM)
	m.HandleFunc("POST /app/im/feishu", b.postIMFeishu)
	m.HandleFunc("GET /app/im/feishu/{id}", b.getIMFeishu)
	m.HandleFunc("DELETE /app/im/feishu/{id}", b.deleteIMFeishu)
	m.HandleFunc("POST /app/im/slack", b.postIMSlack)
	m.HandleFunc("DELETE /app/im/{name}", b.deleteIM)
}

// IMBridge is a bot as the App shows it (no secrets).
type IMBridge struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Agent    string `json:"agent,omitempty"`
	Owner    bool   `json:"has_owner"`
	// Claim: no owner yet; send "/claim <code>" to the bot to become it.
	Claim  string `json:"claim,omitempty"`
	Node   string `json:"node,omitempty"`
	Hub    string `json:"hub,omitempty"`
	Online bool   `json:"online"`
}

// imScan is a QR code being scanned.
type imScan struct {
	mu     sync.Mutex
	state  string // waiting, connecting, done, failed
	err    string
	bridge *IMBridge
	cancel context.CancelFunc
	at     time.Time
}

var imScans sync.Map // id → *imScan

// imRun runs myco im …; a myco from before chat apps says so plainly.
func (b *Backend) imRun(ctx context.Context, args ...string) ([]byte, error) {
	out, err := b.Local.run(ctx, append([]string{"im"}, args...)...)
	var ce *client.Error
	if err != nil && errors.As(err, &ce) && strings.Contains(ce.Message, `unknown command "im"`) {
		return out, &client.Error{Status: http.StatusConflict, Code: "cli_too_old",
			Message:    "this computer's myco command is too old for chat apps",
			Suggestion: "update myco on this computer (Settings › About), then come back"}
	}
	return out, err
}

func (b *Backend) imList(ctx context.Context) ([]IMBridge, error) {
	out, err := b.imRun(ctx, "list")
	if err != nil {
		return nil, err
	}
	var res struct {
		Bridges []struct {
			Name     string `json:"name"`
			Platform string `json:"platform"`
			Agent    string `json:"agent"`
			Owner    string `json:"owner"`
			Claim    bool   `json:"claim_pending"`
			Node     string `json:"node"`
			Hub      string `json:"hub"`
			Online   bool   `json:"online"`
			Profile  string `json:"profile"`
		} `json:"bridges"`
		Dir string `json:"dir"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("reading myco im list: %w", err)
	}
	var list []IMBridge
	for _, r := range res.Bridges {
		br := IMBridge{Name: r.Name, Platform: r.Platform, Agent: r.Agent, Owner: r.Owner != "", Node: r.Node, Hub: r.Hub, Online: r.Online}
		if r.Claim && res.Dir != "" {
			// The code is on this computer only, for its owner to send.
			if c, err := im.Load(res.Dir, r.Name); err == nil {
				br.Claim = c.Claim
			}
		}
		list = append(list, br)
	}
	return list, nil
}

func (b *Backend) getIM(w http.ResponseWriter, r *http.Request) {
	all, err := b.imList(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	hub, agent := b.hubURL(r), r.URL.Query().Get("agent")
	list := []IMBridge{}
	for _, br := range all {
		if (hub == "" || config.SameHub(br.Hub, hub)) && (agent == "" || br.Agent == agent) {
			list = append(list, br)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"bridges": list, "slack_create_url": imslack.CreateURL(orStr(agent, "myco"))})
}

// imConnect runs myco im connect with secrets passed in files that only
// this run reads.
func (b *Backend) imConnect(ctx context.Context, hub string, args []string, secrets map[string]string) (*IMBridge, error) {
	dir, err := os.MkdirTemp("", "myco-im-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	full := append([]string{"connect"}, args...)
	for flag, v := range secrets {
		p := filepath.Join(dir, strings.TrimSuffix(flag, "-file"))
		if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
			return nil, err
		}
		full = append(full, "--"+flag, p)
	}
	if hub != "" {
		full = append(full, "--hub", hub)
	}
	out, err := b.imRun(ctx, full...)
	if err != nil {
		return nil, err
	}
	var res struct {
		Bridge struct {
			Name, Platform, Agent, Owner string
		} `json:"bridge"`
	}
	_ = json.Unmarshal(out, &res)
	br := &IMBridge{Name: res.Bridge.Name, Platform: res.Bridge.Platform, Agent: res.Bridge.Agent, Owner: res.Bridge.Owner != "", Hub: hub}
	if !br.Owner {
		if c, err := im.Load(im.Dir(filepath.Join(b.Local.Home, ".myco")), br.Name); err == nil {
			br.Claim = c.Claim
		}
	}
	return br, nil
}

func (b *Backend) postIMFeishu(w http.ResponseWriter, r *http.Request) {
	var in struct {
		HubID string `json:"hub_id"`
		Agent string `json:"agent"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Agent == "" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "which agent the bot is for (agent) is required"})
		return
	}
	hub, err := b.find(in.HubID)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := (&feishu.Registrar{}).Begin(r.Context())
	if err != nil {
		writeErr(w, &client.Error{Status: http.StatusBadGateway, Code: "unavailable", Message: "Feishu could not be reached: " + err.Error()})
		return
	}
	code, err := qr.Encode(p.URL, qr.M)
	if err != nil {
		writeErr(w, err)
		return
	}
	code.Scale = 8
	idb := make([]byte, 8)
	_, _ = rand.Read(idb)
	id := hex.EncodeToString(idb)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	s := &imScan{state: "waiting", cancel: cancel, at: time.Now()}
	imScans.Store(id, s)
	go func() {
		defer cancel()
		set := func(state, msg string, br *IMBridge) {
			s.mu.Lock()
			s.state, s.err, s.bridge = state, msg, br
			s.mu.Unlock()
		}
		reg, err := p.Wait(ctx)
		if err != nil {
			set("failed", err.Error(), nil)
			return
		}
		set("connecting", "", nil)
		args := []string{reg.Platform, "--agent", in.Agent, "--app-id", reg.AppID, "--owner", reg.OwnerOpenID}
		br, err := b.imConnect(ctx, hub.URL, args, map[string]string{"app-secret-file": reg.AppSecret})
		if err != nil {
			set("failed", errText(err), nil)
			return
		}
		set("done", "", br)
	}()
	// Forget finished scans after a while.
	time.AfterFunc(30*time.Minute, func() { imScans.Delete(id) })
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "url": p.URL,
		"qr": "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG())})
}

func (b *Backend) getIMFeishu(w http.ResponseWriter, r *http.Request) {
	v, ok := imScans.Load(r.PathValue("id"))
	if !ok {
		writeErr(w, &client.Error{Status: http.StatusNotFound, Code: "not_found", Message: "no such QR code (it expired)"})
		return
	}
	s := v.(*imScan)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{"state": s.state}
	if s.err != "" {
		out["error"] = s.err
	}
	if s.bridge != nil {
		out["bridge"] = s.bridge
	}
	writeJSON(w, http.StatusOK, out)
}

func (b *Backend) deleteIMFeishu(w http.ResponseWriter, r *http.Request) {
	if v, ok := imScans.LoadAndDelete(r.PathValue("id")); ok {
		v.(*imScan).cancel()
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopped": true})
}

func (b *Backend) postIMSlack(w http.ResponseWriter, r *http.Request) {
	var in struct {
		HubID    string `json:"hub_id"`
		Agent    string `json:"agent"`
		BotToken string `json:"bot_token"`
		AppToken string `json:"app_token"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.BotToken, in.AppToken = strings.TrimSpace(in.BotToken), strings.TrimSpace(in.AppToken)
	if in.Agent == "" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "which agent the bot is for (agent) is required"})
		return
	}
	if !strings.HasPrefix(in.BotToken, "xoxb-") || !strings.HasPrefix(in.AppToken, "xapp-") {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument",
			Message: "the bot token starts with xoxb- and the app-level token with xapp-"})
		return
	}
	hub, err := b.find(in.HubID)
	if err != nil {
		writeErr(w, err)
		return
	}
	args := []string{"slack", "--agent", in.Agent}
	br, err := b.imConnect(r.Context(), hub.URL, args, map[string]string{"bot-token-file": in.BotToken, "app-token-file": in.AppToken})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bridge": br})
}

func (b *Backend) deleteIM(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !im.ValidName(name) {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "invalid bridge name"})
		return
	}
	if _, err := b.imRun(r.Context(), "remove", "--name", name); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": name})
}
