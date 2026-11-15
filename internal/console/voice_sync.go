package console

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// Two things about voice settings that are not about a call:
//
// The speech service is one of several. Which ones this App can speak with
// is a list here (one today: Doubao); the settings name the one chosen and
// keep a key for each, and the pages draw what this list says, so another
// service is added here and nowhere else.
//
// The settings are the person's, not this computer's. They are kept on the
// hubs the App is signed in to (GET/PUT /v1/console/voice), so the phone
// and another computer have them without setting them up again. The newest
// change wins, by the time the person made it.

// A speech service the voice can speak with.
type ttsProvider struct {
	ID string
	// What people call it.
	Name, NameEn string
	// The voice used when none is chosen, and how to get a key and a voice.
	DefaultSpeaker string
	Help, HelpEn   string
	speech         func(key string, c VoiceConfig) voiceSpeech
}

// VoiceTTSProvider is a speech service as the pages show it.
type VoiceTTSProvider struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	NameEn         string `json:"name_en"`
	DefaultSpeaker string `json:"default_speaker,omitempty"`
	Help           string `json:"help,omitempty"`
	HelpEn         string `json:"help_en,omitempty"`
	// HasKey: this App holds a key for it.
	HasKey bool `json:"has_key"`
}

const defaultTTSProvider = "doubao"

var ttsProviders = []ttsProvider{{
	ID: "doubao", Name: "豆包语音", NameEn: "Doubao Speech",
	DefaultSpeaker: defaultVoiceSpeaker,
	Help:           "在火山引擎控制台「豆包语音」里开通语音合成、创建 API Key。音色填控制台里的音色 ID（复刻的声音以 S_ 开头）。",
	HelpEn:         "Turn on speech synthesis under Doubao Speech in the Volcengine console and create an API key. The voice is a voice ID from the console (cloned voices start with S_).",
	speech: func(key string, c VoiceConfig) voiceSpeech {
		return &doubaoTTS{key: key, speaker: c.TTSSpeaker, rate: c.TTSRate}
	},
}}

// ttsProviderOf finds a speech service; "" is the default one.
func ttsProviderOf(id string) *ttsProvider {
	if id == "" {
		id = defaultTTSProvider
	}
	for i := range ttsProviders {
		if ttsProviders[i].ID == id {
			return &ttsProviders[i]
		}
	}
	return nil
}

// ttsSecret is where a speech service's key is kept in the keychain.
// Doubao's keeps the name it had when it was the only one.
func ttsSecret(provider string) string {
	if provider == "" || provider == defaultTTSProvider {
		return voiceTTSSecret
	}
	return voiceTTSSecret + ":" + provider
}

func (b *Backend) ttsKey(provider string) string {
	k, _ := b.Secrets.Get(ttsSecret(provider))
	return k
}

func (b *Backend) voiceTTSProviders() []VoiceTTSProvider {
	out := make([]VoiceTTSProvider, 0, len(ttsProviders))
	for _, p := range ttsProviders {
		out = append(out, VoiceTTSProvider{ID: p.ID, Name: p.Name, NameEn: p.NameEn, DefaultSpeaker: p.DefaultSpeaker,
			Help: p.Help, HelpEn: p.HelpEn, HasKey: b.ttsKey(p.ID) != ""})
	}
	return out
}

// --- kept on the hubs ---

// VoiceSyncHub says how one hub stands with the voice settings.
type VoiceSyncHub struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// synced | unsupported (a hub from before this) | unreachable
	State string `json:"state"`
}

// voiceSync is the last look at the hubs.
type voiceSync struct {
	mu   sync.Mutex
	hubs []VoiceSyncHub
	at   time.Time
	once sync.Once
}

// voiceDoc is what this App has, as the document the hubs keep: the model
// in use (the translation's when voice has none of its own), the speech
// service, and every key it holds.
func (b *Backend) voiceDoc() api.VoiceSettings {
	c := b.effectiveVoiceConfig()
	llm, _ := b.voiceKeys(c)
	doc := api.VoiceSettings{
		LLMBaseURL: c.LLMBaseURL, LLMModel: c.LLMModel, LLMKey: llm,
		TTSProvider: c.TTSProvider, TTSSpeaker: b.voiceState().stored().TTSSpeaker, TTSRate: c.TTSRate,
		UpdatedAt: c.UpdatedAt,
	}
	for _, p := range ttsProviders {
		if k := b.ttsKey(p.ID); k != "" {
			if doc.TTSKeys == nil {
				doc.TTSKeys = map[string]string{}
			}
			doc.TTSKeys[p.ID] = k
		}
	}
	return doc
}

func voiceDocEmpty(d api.VoiceSettings) bool {
	return d.LLMBaseURL == "" && d.LLMModel == "" && d.LLMKey == "" && d.TTSSpeaker == "" && d.TTSRate == 0 && len(d.TTSKeys) == 0
}

// adoptVoice makes a hub's settings this App's: the choices to the
// settings file, the keys to the keychain. A key the document does not
// have is removed here too (it was removed on another device).
func (b *Backend) adoptVoice(doc api.VoiceSettings) error {
	c := b.voiceState().stored()
	c.LLMBaseURL, c.LLMModel = doc.LLMBaseURL, doc.LLMModel
	c.TTSProvider, c.TTSSpeaker, c.TTSRate = doc.TTSProvider, doc.TTSSpeaker, doc.TTSRate
	c.UpdatedAt = doc.UpdatedAt
	put := func(name, key string) error {
		if key == "" {
			if have, _ := b.Secrets.Get(name); have == "" {
				return nil
			}
			return b.Secrets.Delete(name)
		}
		return b.Secrets.Set(name, key)
	}
	if err := put(voiceLLMSecret, doc.LLMKey); err != nil {
		return err
	}
	for _, p := range ttsProviders {
		if err := put(ttsSecret(p.ID), doc.TTSKeys[p.ID]); err != nil {
			return err
		}
	}
	return b.voiceState().save(c)
}

// syncVoice brings this App and its hubs to the newest voice settings:
// it reads every hub's, takes the newest (a hub's, when newer than this
// App's), and sends it to the hubs that are behind. Off when the person
// turned sharing off. It never fails: a hub that cannot be reached is
// tried again next time.
func (b *Backend) syncVoice(ctx context.Context) []VoiceSyncHub {
	v := b.voiceState()
	if !v.stored().shares() || b.Store == nil {
		v.sync.mu.Lock()
		v.sync.hubs, v.sync.at = nil, time.Now()
		v.sync.mu.Unlock()
		return nil
	}
	hubs, err := b.Store.List()
	if err != nil || len(hubs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	type seen struct {
		c   *client.Client
		doc *api.VoiceSettings
		err error
	}
	got := make([]seen, len(hubs))
	var wg sync.WaitGroup
	for i, h := range hubs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := b.client(h)
			if err != nil {
				got[i].err = err
				return
			}
			got[i].c = c
			got[i].doc, got[i].err = c.VoiceSettings(ctx)
		}()
	}
	wg.Wait()

	local := b.voiceDoc()
	newest, fromHub := local, false
	for _, g := range got {
		if g.err == nil && g.doc != nil && g.doc.UpdatedAt.After(newest.UpdatedAt) {
			newest, fromHub = *g.doc, true
		}
	}
	if fromHub {
		if err := b.adoptVoice(newest); err == nil {
			local = b.voiceDoc()
		}
	} else if local.UpdatedAt.IsZero() && !voiceDocEmpty(local) {
		// Set up before settings were shared: they count as changed now.
		c := v.stored()
		c.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if v.save(c) == nil {
			local.UpdatedAt = c.UpdatedAt
		}
	}
	out := make([]VoiceSyncHub, len(hubs))
	for i, h := range hubs {
		out[i] = VoiceSyncHub{ID: h.ID, Name: h.Name, State: "synced"}
		g := got[i]
		var ce *client.Error
		switch {
		case g.err != nil && errors.As(g.err, &ce) && (ce.Status == http.StatusNotFound || ce.Code == "not_found"):
			out[i].State = "unsupported"
			continue
		case g.err != nil:
			out[i].State = "unreachable"
			continue
		}
		if local.UpdatedAt.IsZero() || !g.doc.UpdatedAt.Before(local.UpdatedAt) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := g.c.PutVoiceSettings(ctx, local); err != nil {
				out[i].State = "unreachable"
			}
		}()
	}
	wg.Wait()
	v.sync.mu.Lock()
	v.sync.hubs, v.sync.at = out, time.Now()
	v.sync.mu.Unlock()
	return out
}

// lastVoiceSync is how the hubs stood at the last look.
func (b *Backend) lastVoiceSync() []VoiceSyncHub {
	v := b.voiceState()
	v.sync.mu.Lock()
	defer v.sync.mu.Unlock()
	return append([]VoiceSyncHub{}, v.sync.hubs...)
}

// syncVoiceRoute is POST /app/voice/sync: look at the hubs now (the
// settings page asks when it opens), then answer as GET /app/voice.
func (b *Backend) syncVoiceRoute(w http.ResponseWriter, r *http.Request) {
	b.syncVoice(r.Context())
	b.getVoice(w, r)
}

func trimVoiceConfig(c *VoiceConfig) {
	c.LLMBaseURL, c.LLMModel = strings.TrimSpace(c.LLMBaseURL), strings.TrimSpace(c.LLMModel)
	c.TTSProvider, c.TTSSpeaker = strings.TrimSpace(c.TTSProvider), strings.TrimSpace(c.TTSSpeaker)
}
