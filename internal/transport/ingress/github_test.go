package ingress

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

func req(event, body, secret string) ([]byte, map[string]string) {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return []byte(body), map[string]string{"X-GitHub-Event": event, "X-Hub-Signature-256": "sha256=" + hex.EncodeToString(m.Sum(nil))}
}

func parse(t *testing.T, g *GitHub, event, body, secret string) ([]string, error) {
	t.Helper()
	b, h := req(event, body, secret)
	r := httptest.NewRequest("POST", "/v1/ingress/github", strings.NewReader(body))
	for k, v := range h {
		r.Header.Set(k, v)
	}
	out, err := g.Parse(r, b)
	var ids []string
	for _, o := range out {
		ids = append(ids, o.MessageID+"|"+o.Source)
	}
	return ids, err
}

func TestGitHubIngress(t *testing.T) {
	g := &GitHub{Secret: "s3cret", AllowedAuthors: []string{"codex-bot"}}
	comment := `{"action":"created","issue":{"title":"[myco] msg_abcdefghijklmnop from alice"},"comment":{"body":"All done","html_url":"u","user":{"login":"codex-bot"}}}`

	if _, err := parse(t, g, "issue_comment", comment, "wrong"); err == nil {
		t.Fatal("bad signature must be rejected")
	}
	ids, err := parse(t, g, "issue_comment", comment, "s3cret")
	if err != nil || len(ids) != 1 || ids[0] != "msg_abcdefghijklmnop|github:codex-bot" {
		t.Fatalf("comment: %v %v", ids, err)
	}
	stranger := strings.Replace(comment, "codex-bot", "random", 1)
	if ids, _ := parse(t, g, "issue_comment", stranger, "s3cret"); len(ids) != 0 {
		t.Fatal("non-allowed author must be ignored")
	}
	own := strings.Replace(comment, "All done", "<!-- myco:hub --> @codex please", 1)
	if ids, _ := parse(t, g, "issue_comment", own, "s3cret"); len(ids) != 0 {
		t.Fatal("hub's own comments must be ignored")
	}
	pr := `{"action":"opened","pull_request":{"title":"Fix","body":"Implements msg_abcdefghijklmnop","html_url":"p","user":{"login":"codex-bot"}}}`
	if ids, _ := parse(t, g, "pull_request", pr, "s3cret"); len(ids) != 1 {
		t.Fatalf("pr: %v", ids)
	}
}
