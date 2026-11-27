package console

import (
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
