package wake

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// Webhook POSTs a signed JSON wake event to any URL. The receiver verifies
// X-Myco-Signature: sha256=HMAC(secret, body), then wakes its agent however
// it likes (e.g. `claude --resume <binding.session_id>` on a VPS).
type Webhook struct{ HTTP Doer }

var _ app.Waker = Webhook{}

func (Webhook) Mode() string   { return "webhook" }
func (Webhook) Consumes() bool { return false }

func (Webhook) Validate(a *domain.Agent) error {
	if err := requireURL(a); err != nil {
		return err
	}
	return requireSecret(a, "an HMAC secret")
}

// WebhookEvent is the JSON body of a webhook wake.
type WebhookEvent struct {
	Event      string    `json:"event"`
	Agent      string    `json:"agent"`
	Harness    string    `json:"harness,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
	Env        string    `json:"environment,omitempty"`
	Workdir    string    `json:"workdir,omitempty"`
	MessageIDs []string  `json:"message_ids"`
	Text       string    `json:"text"`
	SentAt     time.Time `json:"sent_at"`
}

func (w Webhook) Wake(ctx context.Context, req app.WakeRequest) error {
	a := req.Agent
	body, err := json.Marshal(WebhookEvent{
		Event: "wake", Agent: string(a.Name), Harness: a.Binding.Harness, SessionID: a.Binding.SessionID,
		Env: a.Binding.Environment, Workdir: a.Binding.Workdir, MessageIDs: ids(req.Pending),
		Text: req.Text, SentAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = do(ctx, httpOr(w.HTTP), "POST", a.Delivery.URL, map[string]string{
		"X-Myco-Event":     "wake",
		"X-Myco-Signature": Sign(a.Delivery.Secret, body),
	}, body)
	return err
}

// Sign returns the X-Myco-Signature header value for body.
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// Verify checks an X-Myco-Signature header in constant time.
func Verify(secret string, body []byte, header string) bool {
	return hmac.Equal([]byte(Sign(secret, body)), []byte(header))
}

func httpOr(h Doer) Doer {
	if h == nil {
		return DefaultHTTP
	}
	return h
}
