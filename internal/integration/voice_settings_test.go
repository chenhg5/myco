package integration_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
)

// A person's voice chat settings live on the hub: every device of theirs
// reads the same ones, keys included; the newest change wins; someone
// else has their own.
func TestVoiceSettingsPerPerson(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()

	none, err := admin.VoiceSettings(c)
	if err != nil || !none.UpdatedAt.IsZero() || none.LLMKey != "" {
		t.Fatalf("at first: %v %+v", err, none)
	}
	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	set := api.VoiceSettings{
		LLMBaseURL: " https://api.example.com/v1 ", LLMModel: "fast-1", LLMKey: "sk-model",
		TTSProvider: "doubao", TTSSpeaker: "zh_voice", TTSRate: 10,
		TTSKeys: map[string]string{"doubao": "key-doubao", "other": "  ", " ": "x"}, UpdatedAt: at,
	}
	got, err := admin.PutVoiceSettings(c, set)
	if err != nil {
		t.Fatal(err)
	}
	if got.LLMBaseURL != "https://api.example.com/v1" || got.LLMKey != "sk-model" || got.TTSProvider != "doubao" ||
		len(got.TTSKeys) != 1 || got.TTSKeys["doubao"] != "key-doubao" || !got.UpdatedAt.Equal(at) {
		t.Fatalf("kept: %+v", got)
	}
	// Another device of the same person reads all of it.
	link, err := admin.CreatePairing(c)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := h.client("").Pair(c, link.Link[strings.Index(link.Link, "/c/")+3:], "my-phone")
	if err != nil {
		t.Fatal(err)
	}
	phone := h.client(paired.Token)
	read, err := phone.VoiceSettings(c)
	if err != nil || read.LLMKey != "sk-model" || read.TTSKeys["doubao"] != "key-doubao" || read.TTSRate != 10 {
		t.Fatalf("on the other device: %v %+v", err, read)
	}
	// A change made earlier than what the hub holds changes nothing: the
	// answer is what it holds.
	older := set
	older.LLMModel, older.UpdatedAt = "old-model", at.Add(-time.Minute)
	if got, err := phone.PutVoiceSettings(c, older); err != nil || got.LLMModel != "fast-1" {
		t.Fatalf("an older change: %v %+v", err, got)
	}
	// A later one from the phone is what the first device reads next.
	newer := set
	newer.LLMModel, newer.TTSKeys, newer.UpdatedAt = "fast-2", nil, at.Add(time.Minute)
	if _, err := phone.PutVoiceSettings(c, newer); err != nil {
		t.Fatal(err)
	}
	if got, _ := admin.VoiceSettings(c); got.LLMModel != "fast-2" || len(got.TTSKeys) != 0 {
		t.Fatalf("after the phone changed it: %+v", got)
	}
	// Someone else has settings of their own.
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "bob-mac")
	if err != nil {
		t.Fatal(err)
	}
	bob := h.client(theirs.Token)
	if got, err := bob.VoiceSettings(c); err != nil || !got.UpdatedAt.IsZero() || got.LLMKey != "" {
		t.Fatalf("someone else's settings: %v %+v", err, got)
	}
	// What cannot be kept is refused; a clock far ahead does not win for ever.
	if _, err := admin.PutVoiceSettings(c, api.VoiceSettings{LLMModel: "m"}); err == nil {
		t.Fatal("a change without its time should be refused")
	}
	if _, err := admin.PutVoiceSettings(c, api.VoiceSettings{TTSRate: 500, UpdatedAt: time.Now()}); err == nil {
		t.Fatal("a speed out of range should be refused")
	}
	ahead, err := admin.PutVoiceSettings(c, api.VoiceSettings{LLMModel: "from-the-future", UpdatedAt: time.Now().Add(48 * time.Hour)})
	if err != nil || ahead.UpdatedAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("a clock ahead: %v %+v", err, ahead.UpdatedAt)
	}
	// An agent's token is not a person's: it cannot read them.
	if _, err := h.client("myco_agent_nope").VoiceSettings(c); err == nil {
		t.Fatal("without a person's token there is nothing to read")
	}
}
