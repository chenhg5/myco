package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
)

// A hub that keeps one person's voice settings, as the real one does
// (newest wins), or one from before that (404).
type settingsHub struct {
	mu   sync.Mutex
	doc  api.VoiceSettings
	old  bool
	puts int
}

func (h *settingsHub) serve(t *testing.T) string {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/console/voice" || h.old {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found","message":"no such route"}`))
			return
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		if r.Method == http.MethodPut {
			var in api.VoiceSettings
			_ = json.NewDecoder(r.Body).Decode(&in)
			h.puts++
			if !h.doc.UpdatedAt.After(in.UpdatedAt) {
				h.doc = in
			}
		}
		_ = json.NewEncoder(w).Encode(h.doc)
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func (h *settingsHub) get() api.VoiceSettings {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.doc
}

func TestVoiceSettingsAreKeptOnTheHubs(t *testing.T) {
	keys := &memKeys{m: map[string]string{}}
	b := &Backend{Store: NewStore(t.TempDir() + "/hubs.json"), Secrets: keys}
	mine, team, older := &settingsHub{}, &settingsHub{}, &settingsHub{old: true}
	for name, h := range map[string]*settingsHub{"mine": mine, "team": team, "older": older} {
		hub, err := b.Store.Add(name, h.serve(t))
		if err != nil {
			t.Fatal(err)
		}
		keys.m[hub.ID] = "myco_console_" + name
	}
	srv := httptestServer(t, b)

	// The speech services come from the backend: the page draws these.
	code, body := srv("GET", "/app/voice", "")
	if code != 200 || !strings.Contains(body, `"tts_providers":[{"id":"doubao"`) || !strings.Contains(body, `"tts_provider":"doubao"`) || !strings.Contains(body, `"share":true`) {
		t.Fatalf("get: %d %s", code, body)
	}
	// One the backend does not know is refused.
	if code, body := srv("PUT", "/app/voice/config", `{"tts_provider":"nope"}`); code != 400 || !strings.Contains(body, "unknown speech service") {
		t.Fatalf("unknown service: %d %s", code, body)
	}
	// Saved here: on every hub that can keep it, keys and all.
	code, body = srv("PUT", "/app/voice/config", `{"llm_base_url":"https://api.example.com/v1","llm_model":"fast-1","llm_key":"sk-model","tts_provider":"doubao","tts_speaker":"zh_x","tts_key":"tts-key"}`)
	if code != 200 || strings.Contains(body, "sk-model") || strings.Contains(body, "tts-key") {
		t.Fatalf("put: %d %s", code, body)
	}
	for name, h := range map[string]*settingsHub{"mine": mine, "team": team} {
		d := h.get()
		if d.LLMModel != "fast-1" || d.LLMKey != "sk-model" || d.TTSProvider != "doubao" || d.TTSSpeaker != "zh_x" || d.TTSKeys["doubao"] != "tts-key" || d.UpdatedAt.IsZero() {
			t.Fatalf("hub %s holds %+v", name, d)
		}
	}
	states := map[string]string{}
	var info struct {
		Sync []VoiceSyncHub `json:"sync"`
	}
	_ = json.Unmarshal([]byte(body), &info)
	for _, s := range info.Sync {
		states[s.Name] = s.State
	}
	if states["mine"] != "synced" || states["team"] != "synced" || states["older"] != "unsupported" {
		t.Fatalf("how the hubs stand: %v", states)
	}

	// Changed on the phone since: this App takes it from the hub, keys
	// too, and brings the other hub up to date.
	phone := mine.get()
	phone.LLMModel, phone.LLMKey, phone.TTSRate = "fast-2", "sk-phone", 20
	phone.TTSKeys = map[string]string{"doubao": "tts-phone"}
	phone.UpdatedAt = time.Now().Add(time.Minute).UTC().Truncate(time.Second)
	mine.mu.Lock()
	mine.doc = phone
	mine.mu.Unlock()
	code, body = srv("POST", "/app/voice/sync", "")
	if code != 200 || !strings.Contains(body, `"llm_model":"fast-2"`) || !strings.Contains(body, `"tts_rate":20`) || strings.Contains(body, "sk-phone") {
		t.Fatalf("sync: %d %s", code, body)
	}
	if keys.m[voiceLLMSecret] != "sk-phone" || keys.m[ttsSecret("doubao")] != "tts-phone" {
		t.Fatalf("keys after the sync: %v", keys.m)
	}
	if d := team.get(); d.LLMModel != "fast-2" || d.LLMKey != "sk-phone" {
		t.Fatalf("the other hub: %+v", d)
	}
	// Nothing new: nothing is sent again.
	before := mine.puts + team.puts
	srv("POST", "/app/voice/sync", "")
	if mine.puts+team.puts != before {
		t.Fatalf("a sync with nothing new sent %d more", mine.puts+team.puts-before)
	}

	// Sharing turned off: a change stays on this computer.
	code, body = srv("PUT", "/app/voice/config", `{"llm_base_url":"https://api.example.com/v1","llm_model":"local-only","tts_provider":"doubao","share":false}`)
	if code != 200 || !strings.Contains(body, `"share":false`) || !strings.Contains(body, `"sync":[]`) {
		t.Fatalf("sharing off: %d %s", code, body)
	}
	if d := mine.get(); d.LLMModel != "fast-2" {
		t.Fatalf("with sharing off the hub got %+v", d)
	}
	// And it stays off until said otherwise.
	srv("PUT", "/app/voice/config", `{"llm_base_url":"https://api.example.com/v1","llm_model":"still-local","tts_provider":"doubao"}`)
	if d := mine.get(); d.LLMModel != "fast-2" {
		t.Fatalf("sharing came back by itself: %+v", d)
	}
}

// Settings made before they were shared count as changed when first
// shared; a hub that already has some (from another device) wins.
func TestVoiceSettingsFromBeforeSharing(t *testing.T) {
	keys := &memKeys{m: map[string]string{voiceLLMSecret: "sk-old", voiceTTSSecret: "tts-old"}}
	b := &Backend{Store: NewStore(t.TempDir() + "/hubs.json"), Secrets: keys}
	_ = b.voiceState().save(VoiceConfig{LLMBaseURL: "https://api.example.com/v1", LLMModel: "old-model"})
	empty, set := &settingsHub{}, &settingsHub{}
	hub, _ := b.Store.Add("empty", empty.serve(t))
	keys.m[hub.ID] = "t"
	srv := httptestServer(t, b)
	srv("POST", "/app/voice/sync", "")
	if d := empty.get(); d.LLMModel != "old-model" || d.LLMKey != "sk-old" || d.TTSKeys["doubao"] != "tts-old" || d.UpdatedAt.IsZero() {
		t.Fatalf("first shared: %+v", d)
	}

	// Another computer, also set up before: the hub's are newer than "never".
	keys2 := &memKeys{m: map[string]string{voiceLLMSecret: "sk-here"}}
	b2 := &Backend{Store: NewStore(t.TempDir() + "/hubs.json"), Secrets: keys2}
	set.doc = api.VoiceSettings{LLMBaseURL: "https://api.example.com/v1", LLMModel: "from-the-phone", LLMKey: "sk-phone", TTSProvider: "doubao", UpdatedAt: time.Now().Add(-time.Hour).UTC()}
	hub2, _ := b2.Store.Add("set", set.serve(t))
	keys2.m[hub2.ID] = "t"
	srv2 := httptestServer(t, b2)
	if _, body := srv2("POST", "/app/voice/sync", ""); !strings.Contains(body, `"llm_model":"from-the-phone"`) || keys2.m[voiceLLMSecret] != "sk-phone" {
		t.Fatalf("a hub that has them: %s %v", body, keys2.m)
	}
}

// Keys that call back when one is set: a look at the hubs can be started
// in the middle of a save.
type hookKeys struct {
	*memKeys
	onSet func(id string)
}

func (s *hookKeys) Set(id, v string) error {
	err := s.memKeys.Set(id, v)
	if s.onSet != nil {
		s.onSet(id)
	}
	return err
}

// A look at the hubs that starts while a save is half done (the keys are
// in, the choices not yet) waits for it: the hub never gets the keys with
// empty choices under the save's own time.
func TestVoiceSaveAndSyncDoNotCross(t *testing.T) {
	mem := &memKeys{m: map[string]string{}}
	keys := &hookKeys{memKeys: mem}
	b := &Backend{Store: NewStore(t.TempDir() + "/hubs.json"), Secrets: keys}
	kept := &settingsHub{}
	hub, err := b.Store.Add("mine", kept.serve(t))
	if err != nil {
		t.Fatal(err)
	}
	mem.m[hub.ID] = "myco_console_mine"
	srv := httptestServer(t, b)

	done := make(chan struct{})
	var once sync.Once
	keys.onSet = func(id string) {
		if id != ttsSecret("doubao") {
			return
		}
		once.Do(func() {
			go func() {
				defer close(done)
				b.syncVoice(context.Background())
			}()
			// Long enough for the look to read the hub and this App's
			// settings, were nothing holding it back.
			time.Sleep(150 * time.Millisecond)
		})
	}
	code, body := srv("PUT", "/app/voice/config", `{"llm_base_url":"https://api.example.com/v1","llm_model":"fast-1","llm_key":"sk-model","tts_provider":"doubao","tts_key":"tts-key"}`)
	if code != 200 {
		t.Fatalf("put: %d %s", code, body)
	}
	<-done
	if d := kept.get(); d.LLMModel != "fast-1" || d.LLMBaseURL == "" || d.LLMKey != "sk-model" || d.TTSKeys["doubao"] != "tts-key" {
		t.Fatalf("the hub holds a half-saved change: model %q, address set %v", d.LLMModel, d.LLMBaseURL != "")
	}

	// Two changes within one second: the second is still the newer one.
	first := kept.get().UpdatedAt
	srv("PUT", "/app/voice/config", `{"llm_base_url":"https://api.example.com/v1","llm_model":"fast-2","tts_provider":"doubao"}`)
	if d := kept.get(); d.LLMModel != "fast-2" || !d.UpdatedAt.After(first) {
		t.Fatalf("a second change in the same second: model %q, %v then %v", d.LLMModel, first, d.UpdatedAt)
	}
}
