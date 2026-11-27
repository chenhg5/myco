package console

import (
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/im"
)

// A bot gets the voice chats' model as its front desk, once, unless its
// owner turned that off; the key goes to the bot's 0600 file only.
func TestIMDeskFromVoice(t *testing.T) {
	keys := &memKeys{m: map[string]string{}}
	b := &Backend{Store: NewStore(t.TempDir() + "/hubs.json"), Secrets: keys}
	dir := t.TempDir()
	c := &im.Config{Name: "feishu-coder", Platform: "feishu", Agent: "coder", Profile: "p"}
	if b.imDesk(dir, c) {
		t.Fatal("no voice model yet")
	}
	srv := httptestServer(t, b)
	if code, body := srv("PUT", "/app/voice/config", `{"llm_base_url":"https://api.example.com/v1","llm_model":"fast","llm_key":"sk-1"}`); code != 200 {
		t.Fatalf("put: %d %s", code, body)
	}
	if !b.imDesk(dir, c) {
		t.Fatal("want the desk")
	}
	got, err := im.Load(dir, "feishu-coder")
	if err != nil || got.LLMModel != "fast" || got.LLMKey != "sk-1" || got.LLMBaseURL != "https://api.example.com/v1" {
		t.Fatalf("saved %+v %v", got, err)
	}
	off := &im.Config{Name: "slack-coder", Platform: "slack", Agent: "coder", Profile: "p", NoDesk: true}
	if b.imDesk(dir, off) || off.LLMKey != "" {
		t.Fatal("turned off by its owner")
	}
}

// The owner's own instructions for a bot: written to its config here.
func TestIMPrompt(t *testing.T) {
	home := t.TempDir()
	b := &Backend{Store: NewStore(t.TempDir() + "/hubs.json"), Secrets: &memKeys{m: map[string]string{}}, Local: &Local{Home: home}}
	dir := im.Dir(home + "/.myco")
	if err := im.Save(dir, &im.Config{Name: "feishu-coder", Platform: "feishu", AppID: "a", AppSecret: "s", Agent: "coder", Profile: "p"}); err != nil {
		t.Fatal(err)
	}
	srv := httptestServer(t, b)
	if code, body := srv("PUT", "/app/im/feishu-coder/prompt", `{"prompt":"  只回答产品问题  "}`); code != 200 || !strings.Contains(body, `"prompt":"只回答产品问题"`) {
		t.Fatalf("put: %d %s", code, body)
	}
	if c, _ := im.Load(dir, "feishu-coder"); c.Prompt != "只回答产品问题" || c.AppSecret != "s" {
		t.Fatalf("saved %+v", c)
	}
	if code, _ := srv("PUT", "/app/im/nobody/prompt", `{"prompt":"x"}`); code != 404 {
		t.Fatalf("unknown bot: %d", code)
	}
	if code, _ := srv("PUT", "/app/im/feishu-coder/prompt", `{"prompt":"`+strings.Repeat("长", im.MaxPrompt+1)+`"}`); code != 400 {
		t.Fatalf("too long: %d", code)
	}
}
