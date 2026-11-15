package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chenhg5/myco/internal/client"
)

// Talking with an agent by voice (docs/design/voice.md), in the person's
// chat with it: the App hears the person (speech recognition), a fast text
// model reads the chat and talks with them, passing what they want done to
// the agent as they would type it (a new task, or an addition to the one at
// work), and answers out loud (Doubao speech synthesis), all streaming. It
// stops talking as soon as the person speaks, and tells them, in a pause,
// when the agent has answered or is asking something.
//
// The microphone and the speaker are the platform's (VoiceAudio, the Mac
// app's: one audio engine with the system's voice processing, so its own
// voice is not heard as the person's and background noise is held down).
//
//	GET  /app/voice                 {available, ready, config, has_llm_key, has_tts_key, tts_providers, share, sync, session}
//	PUT  /app/voice/config          {llm_base_url, llm_model, llm_key?, tts_provider, tts_speaker, tts_key?, share?, clear_*}
//	POST /app/voice/sync            looks at the hubs for newer settings, then answers as GET
//	POST /app/voice/start           {hub_id, agent, peer?, lang}  starts a call with one agent
//	POST /app/voice/stop            hangs up
//	POST /app/voice/mute            {muted}
//	GET  /app/voice/events?after=N  long-poll: {state, events, seq}

// VoiceAudio is the platform's microphone and speaker for a voice call.
type VoiceAudio interface {
	// Start opens the microphone (with echo cancellation and noise
	// suppression) and recognises speech in lang (a BCP 47 tag); what it
	// hears goes to ev. One call at a time.
	Start(lang string, ev VoiceEvents) error
	// Play queues 16-bit little-endian mono PCM at rate Hz.
	Play(pcm []byte, rate int)
	// Flush stops what is playing and drops what is queued.
	Flush()
	// Stop closes the microphone and the speaker.
	Stop()
}

// VoiceEvents is what the platform tells the call.
type VoiceEvents interface {
	// Heard is the person's words so far in this utterance; final when
	// the recognizer has settled it.
	Heard(text string, final bool)
	// Played says the speaker has played everything queued.
	Played()
	// Ended: the microphone stopped (err says why, "" when asked to).
	Ended(err string)
}

// VoiceConfig is what the person chose for voice (no keys).
type VoiceConfig struct {
	LLMBaseURL string `json:"llm_base_url,omitempty"`
	LLMModel   string `json:"llm_model,omitempty"`
	// TTSProvider: the speech service the voice speaks with (voice_sync.go
	// lists them); "" is the default one.
	TTSProvider string `json:"tts_provider,omitempty"`
	TTSSpeaker  string `json:"tts_speaker,omitempty"`
	// TTSRate: how fast it talks, -50..100 (0 = normal).
	TTSRate int `json:"tts_rate,omitempty"`
	// Share: keep these settings (keys too) on the hubs this App is signed
	// in to, so the person's other devices have them. On unless turned off.
	Share *bool `json:"share,omitempty"`
	// UpdatedAt: when the person last changed them (on any device).
	UpdatedAt time.Time `json:"updated_at"`
}

// shares says whether the settings are kept on the hubs.
func (c VoiceConfig) shares() bool { return c.Share == nil || *c.Share }

const (
	defaultVoiceSpeaker = "zh_male_wennuanahu_uranus_bigtts"
	voiceLLMSecret      = "voice:llm"
	voiceTTSSecret      = "voice:tts"
)

// Voice event kinds.
const (
	vUser      = "user"      // the person said (final)
	vHearing   = "hearing"   // the person is saying (partial; not kept)
	vAssistant = "assistant" // the assistant said
	vAction    = "action"    // it did something in myco
	vNotice    = "notice"    // something to know (an error, a hint)
)

// VoiceEvent is one line of the call, as the page shows it.
type VoiceEvent struct {
	// Seq orders changes (a line said bit by bit gets a new one each
	// time); ID stays the line's own, so the page replaces it.
	Seq  int    `json:"seq"`
	ID   int    `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
	// Interrupted: the assistant was cut off; Text is what it got to say.
	Interrupted bool `json:"interrupted,omitempty"`
	// Task links an action to the task it made or touched.
	Task *VoiceTaskRef `json:"task,omitempty"`
	At   time.Time     `json:"at"`
}

// VoiceTaskRef names a task in a workspace.
type VoiceTaskRef struct {
	HubID  string `json:"hub_id"`
	ID     string `json:"id"`
	Agent  string `json:"agent,omitempty"`
	Thread string `json:"thread_id,omitempty"`
}

// voiceWith: the chat a call belongs to.
type voiceWith struct {
	HubID string `json:"hub_id"`
	Agent string `json:"agent"`
}

// Voice call states.
const (
	stIdle      = "idle"      // no call
	stListening = "listening" // waiting for the person
	stThinking  = "thinking"  // the model is working on an answer
	stSpeaking  = "speaking"  // the answer is being said
)

// voice is the App's voice assistant: settings and at most one call.
type voice struct {
	mu      sync.Mutex
	path    string // settings file
	logPath string // what was said, kept on this computer
	call    *voiceCall
	with    voiceWith // whom the call is with (while on)
	events  []VoiceEvent
	seq     int
	state   string
	changed chan struct{}
	// The settings, when there is no file to keep them in (tests).
	mem VoiceConfig
	// How the hubs stood with the settings at the last look.
	sync voiceSync
}

func (b *Backend) voiceState() *voice {
	b.voiceOnce.Do(func() {
		v := &voice{state: stIdle, changed: make(chan struct{})}
		if b.Store != nil {
			dir := filepath.Dir(b.Store.path)
			v.path = filepath.Join(dir, "console-voice.json")
			v.logPath = filepath.Join(dir, "voice-log.json")
			v.loadLog()
		}
		b.voice = v
	})
	return b.voice
}

// stored is what the settings file says, as it is.
func (v *voice) stored() VoiceConfig {
	var c VoiceConfig
	if v.path != "" {
		if b, err := os.ReadFile(v.path); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	} else {
		v.mu.Lock()
		c = v.mem
		v.mu.Unlock()
	}
	return c
}

// config is the settings in use: the default speech service and its
// default voice where none is chosen.
func (v *voice) config() VoiceConfig {
	c := v.stored()
	if ttsProviderOf(c.TTSProvider) == nil || c.TTSProvider == "" {
		c.TTSProvider = defaultTTSProvider
	}
	if c.TTSSpeaker == "" {
		c.TTSSpeaker = ttsProviderOf(c.TTSProvider).DefaultSpeaker
	}
	return c
}

func (v *voice) save(c VoiceConfig) error {
	if v.path == "" {
		// No settings file (tests): kept in memory.
		v.mu.Lock()
		v.mem = c
		v.mu.Unlock()
		return nil
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return writeFileAtomic(v.path, b)
}

// The transcript kept on this computer: what the person and the assistant
// said and did (not the partial hearing), the last maxVoiceLog lines.
const maxVoiceLog = 400

func (v *voice) loadLog() {
	b, err := os.ReadFile(v.logPath)
	if err != nil {
		return
	}
	var evs []VoiceEvent
	if json.Unmarshal(b, &evs) == nil {
		v.events = evs
		if n := len(evs); n > 0 {
			v.seq = evs[n-1].Seq
		}
	}
}

func (v *voice) saveLog() {
	if v.logPath == "" {
		return
	}
	keep := make([]VoiceEvent, 0, len(v.events))
	for _, e := range v.events {
		if e.Kind != vHearing {
			keep = append(keep, e)
		}
	}
	if len(keep) > maxVoiceLog {
		keep = keep[len(keep)-maxVoiceLog:]
	}
	b, _ := json.Marshal(keep)
	_ = writeFileAtomic(v.logPath, b)
}

// add records an event (a partial hearing replaces the one before it).
// Holds v.mu.
func (v *voice) add(e VoiceEvent) VoiceEvent {
	if n := len(v.events); e.Kind == vHearing && n > 0 && v.events[n-1].Kind == vHearing {
		v.events = v.events[:n-1]
	}
	if n := len(v.events); e.Kind == vUser && n > 0 && v.events[n-1].Kind == vHearing {
		v.events = v.events[:n-1]
	}
	v.seq++
	e.Seq, e.ID = v.seq, v.seq
	if e.At.IsZero() {
		e.At = time.Now()
	}
	v.events = append(v.events, e)
	if len(v.events) > maxVoiceLog+50 {
		v.events = v.events[len(v.events)-maxVoiceLog:]
	}
	if e.Kind != vHearing {
		v.saveLog()
	}
	v.wakeLocked()
	return e
}

// update changes the text of line id (an answer still being said).
// Holds v.mu.
func (v *voice) update(id int, text string, interrupted bool) {
	for i := len(v.events) - 1; i >= 0; i-- {
		if v.events[i].ID == id {
			v.seq++
			e := v.events[i]
			e.Text, e.Interrupted = text, interrupted
			// Moved to the end with a new seq, so pollers see it change.
			v.events = append(v.events[:i], v.events[i+1:]...)
			e.Seq = v.seq
			v.events = append(v.events, e)
			v.saveLog()
			v.wakeLocked()
			return
		}
	}
}

func (v *voice) setState(s string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.state != s {
		v.state = s
		v.wakeLocked()
	}
}

func (v *voice) wakeLocked() {
	close(v.changed)
	v.changed = make(chan struct{})
}

func (b *Backend) voiceRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/voice", b.getVoice)
	m.HandleFunc("PUT /app/voice/config", b.putVoiceConfig)
	m.HandleFunc("POST /app/voice/sync", b.syncVoiceRoute)
	m.HandleFunc("POST /app/voice/start", b.startVoice)
	m.HandleFunc("POST /app/voice/stop", b.stopVoice)
	m.HandleFunc("POST /app/voice/mute", b.muteVoice)
	m.HandleFunc("GET /app/voice/events", b.voiceEvents)
}

// voiceKeys says which keys are saved: the model's (its own, or the
// translation's OpenAI-compatible one at the same address) and Doubao's.
func (b *Backend) voiceKeys(c VoiceConfig) (llm, tts string) {
	llm, _ = b.Secrets.Get(voiceLLMSecret)
	if llm == "" {
		tc := b.translatorState().config()
		if tc.Provider == "openai" && (c.LLMBaseURL == "" || strings.TrimRight(c.LLMBaseURL, "/") == strings.TrimRight(tc.BaseURL, "/")) {
			llm, _ = b.Secrets.Get(secretName("openai"))
		}
	}
	tts = b.ttsKey(c.TTSProvider)
	return llm, tts
}

// effectiveConfig fills the model from the translation settings when voice
// has none of its own.
func (b *Backend) effectiveVoiceConfig() VoiceConfig {
	c := b.voiceState().config()
	if c.LLMBaseURL == "" || c.LLMModel == "" {
		if tc := b.translatorState().config(); tc.Provider == "openai" {
			if c.LLMBaseURL == "" {
				c.LLMBaseURL = tc.BaseURL
			}
			if c.LLMModel == "" {
				c.LLMModel = tc.Model
			}
		}
	}
	return c
}

func (b *Backend) getVoice(w http.ResponseWriter, r *http.Request) {
	v := b.voiceState()
	c := b.effectiveVoiceConfig()
	llm, tts := b.voiceKeys(c)
	v.mu.Lock()
	state, on := v.state, v.call != nil
	muted := on && v.call.muted
	session := map[string]any{"on": on, "state": state, "muted": muted}
	if on {
		session["hub_id"], session["agent"] = v.with.HubID, v.with.Agent
	}
	v.mu.Unlock()
	// ready: a call can start (this App has a microphone, and the model and
	// the voice are set up); the App shows the way in only then.
	ready := b.Voice != nil && c.LLMBaseURL != "" && c.LLMModel != "" && llm != "" && tts != ""
	// The first look at the hubs for the settings kept there, in the
	// background: a device set up elsewhere becomes ready by itself.
	v.sync.once.Do(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			b.syncVoice(ctx)
		}()
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"available": b.Voice != nil, "ready": ready, "config": c, "has_llm_key": llm != "", "has_tts_key": tts != "",
		"session": session,
		// The speech services this App can speak with (the pages draw
		// these), and how the hubs stand with the settings.
		"tts_providers": b.voiceTTSProviders(), "share": c.shares(), "sync": b.lastVoiceSync(),
	})
}

func (b *Backend) putVoiceConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		VoiceConfig
		LLMKey   string `json:"llm_key"`
		TTSKey   string `json:"tts_key"`
		ClearLLM bool   `json:"clear_llm_key"`
		ClearTTS bool   `json:"clear_tts_key"`
	}
	if !decode(w, r, &in) {
		return
	}
	c := in.VoiceConfig
	trimVoiceConfig(&c)
	if c.TTSRate < -50 || c.TTSRate > 100 {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "speech rate must be between -50 and 100"})
		return
	}
	if ttsProviderOf(c.TTSProvider) == nil {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "unknown speech service " + c.TTSProvider,
			Suggestion: "GET /app/voice lists the speech services under tts_providers"})
		return
	}
	// The key typed is the chosen speech service's. Sharing stays as it
	// was unless this change says otherwise.
	if c.Share == nil {
		c.Share = b.voiceState().stored().Share
	}
	c.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	set := func(name, key string, clear bool) error {
		switch {
		case clear:
			return b.Secrets.Delete(name)
		case strings.TrimSpace(key) != "":
			return b.Secrets.Set(name, strings.TrimSpace(key))
		}
		return nil
	}
	if err := set(voiceLLMSecret, in.LLMKey, in.ClearLLM); err != nil {
		writeErr(w, err)
		return
	}
	if err := set(ttsSecret(c.TTSProvider), in.TTSKey, in.ClearTTS); err != nil {
		writeErr(w, err)
		return
	}
	if err := b.voiceState().save(c); err != nil {
		writeErr(w, err)
		return
	}
	// The person's other devices get it from the hubs.
	b.syncVoice(r.Context())
	b.getVoice(w, r)
}

func (b *Backend) startVoice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		HubID string `json:"hub_id"`
		Agent string `json:"agent"`
		Peer  string `json:"peer"`
		Lang  string `json:"lang"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Agent = strings.TrimSpace(in.Agent)
	if in.Agent == "" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "agent is required: a voice call is with one agent"})
		return
	}
	if b.Voice == nil {
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "unsupported", Message: "voice calls need the desktop app on a Mac for now"})
		return
	}
	hub, err := b.find(in.HubID)
	if err != nil {
		writeErr(w, err)
		return
	}
	c := b.effectiveVoiceConfig()
	llmKey, ttsKey := b.voiceKeys(c)
	if c.LLMBaseURL == "" || c.LLMModel == "" || llmKey == "" || ttsKey == "" {
		// Maybe set up on another device since: look at the hubs once more.
		b.syncVoice(r.Context())
		c = b.effectiveVoiceConfig()
		llmKey, ttsKey = b.voiceKeys(c)
	}
	provider := ttsProviderOf(c.TTSProvider)
	if c.LLMBaseURL == "" || c.LLMModel == "" || llmKey == "" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "not_configured", Message: "choose the model the voice assistant talks with",
			Suggestion: "设置 › 语音：填 OpenAI 兼容接口的地址、模型和密钥（或在「翻译」里配好 OpenAI 兼容接口）"})
		return
	}
	if provider == nil || ttsKey == "" {
		name := c.TTSProvider
		if provider != nil {
			name = provider.Name
		}
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "not_configured", Message: "add the speech service's key (" + c.TTSProvider + ")",
			Suggestion: "设置 › 语音：填语音合成（" + name + "）的 API Key"})
		return
	}
	hc, err := b.client(hub)
	if err != nil {
		writeErr(w, err)
		return
	}
	lang := strings.TrimSpace(in.Lang)
	if lang == "" {
		lang = "zh-CN"
	}
	peer := strings.TrimSpace(in.Peer)
	if peer == "" {
		if peer, err = consoleAddress(r.Context(), hc); err != nil {
			writeErr(w, err)
			return
		}
	}
	v := b.voiceState()
	v.mu.Lock()
	if v.call != nil {
		v.mu.Unlock()
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "conflict", Message: "a voice call is already on"})
		return
	}
	call := newVoiceCall(v, voiceDeps{
		audio: b.Voice,
		model: &openAIModel{base: c.LLMBaseURL, model: c.LLMModel, key: llmKey, session: "myco-voice-" + requestID()},
		tts:   provider.speech(ttsKey, c),
		hub:   &hubActions{hubID: hub.ID, agent: in.Agent, peer: peer, c: hc},
		lang:  lang,
	})
	v.call = call
	v.with = voiceWith{HubID: hub.ID, Agent: in.Agent}
	v.mu.Unlock()
	if err := b.Voice.Start(lang, call); err != nil {
		v.mu.Lock()
		v.call = nil
		v.mu.Unlock()
		writeErr(w, err)
		return
	}
	call.begin()
	b.getVoice(w, r)
}

// consoleAddress: the person's own address in a workspace (what their
// messages from the App are from).
func consoleAddress(ctx context.Context, c *client.Client) (string, error) {
	who, err := c.Whoami(ctx)
	if err != nil {
		return "", err
	}
	switch {
	case who.Member != nil:
		return "@console:" + who.Member.Name, nil
	case who.Device != nil:
		return "@console:" + who.Device.Name, nil
	}
	return "@console:admin", nil
}

func (b *Backend) stopVoice(w http.ResponseWriter, r *http.Request) {
	v := b.voiceState()
	v.mu.Lock()
	call := v.call
	v.mu.Unlock()
	if call != nil {
		call.hangUp()
	}
	b.getVoice(w, r)
}

func (b *Backend) muteVoice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Muted bool `json:"muted"`
	}
	if !decode(w, r, &in) {
		return
	}
	v := b.voiceState()
	v.mu.Lock()
	if v.call != nil {
		v.call.muted = in.Muted
	}
	v.wakeLocked()
	v.mu.Unlock()
	b.getVoice(w, r)
}

// voiceEvents: the lines after `after`, waiting up to 25 seconds for one.
func (b *Backend) voiceEvents(w http.ResponseWriter, r *http.Request) {
	v := b.voiceState()
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	for {
		v.mu.Lock()
		var out []VoiceEvent
		for _, e := range v.events {
			if e.Seq > after {
				out = append(out, e)
			}
		}
		state, on, seq, ch := v.state, v.call != nil, v.seq, v.changed
		muted := on && v.call.muted
		v.mu.Unlock()
		if len(out) > 0 || after < 0 {
			writeJSON(w, http.StatusOK, map[string]any{"state": state, "on": on, "muted": muted, "seq": seq, "events": nonNilEvents(out)})
			return
		}
		select {
		case <-ch:
			// a new line, or the state changed: answer if it was the state
			v.mu.Lock()
			changedState := v.state != state || (v.call != nil) != on
			v.mu.Unlock()
			if changedState {
				writeJSON(w, http.StatusOK, map[string]any{"state": v.state, "on": v.call != nil, "seq": seq, "events": []VoiceEvent{}})
				return
			}
		case <-deadline.C:
			writeJSON(w, http.StatusOK, map[string]any{"state": state, "on": on, "seq": seq, "events": []VoiceEvent{}})
			return
		case <-r.Context().Done():
			return
		}
	}
}

func nonNilEvents(e []VoiceEvent) []VoiceEvent {
	if e == nil {
		return []VoiceEvent{}
	}
	return e
}

// clip shortens s to n characters.
func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

var errVoiceStopped = errors.New("voice call ended")

// writeFileAtomic writes b to path (mode 600) through a temporary file.
func writeFileAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
