package console

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/client"
)

// The voices a speech service offers, for the settings to choose from.
//
// Doubao can list a tenant's voices (ListSpeakers), but only with a
// Volcengine account's signing keys, not the speech API key kept here; so
// the list is the official one for its 2.0 model (docs: 豆包语音 → 在线音色
// 列表), shipped with the App. A voice not on it (a cloned one, S_...) is
// still typed in by hand.

// TTSVoice is one voice on a speech service's list.
type TTSVoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// The language it is made for: zh, en, ja, ... (Doubao's id prefix; mx is
	// Mexican Spanish, tl Filipino).
	Lang string `json:"lang"`
	// female | male
	Gender string `json:"gender"`
}

//go:embed voice_doubao_voices.json
var doubaoVoicesJSON []byte

var doubaoVoices = func() []TTSVoice {
	var v []TTSVoice
	if err := json.Unmarshal(doubaoVoicesJSON, &v); err != nil {
		panic("voice_doubao_voices.json: " + err.Error())
	}
	return v
}()

// What a voice says when it is tried, in its own language (English when
// there is no line for it).
var voiceSampleLines = map[string]string{
	"zh": "你好，有什么要我做的，直接跟我说就行。",
	"en": "Hi, just tell me what you'd like me to do.",
	"ja": "こんにちは。やってほしいことを、そのまま話してください。",
	"ko": "안녕하세요, 제가 할 일을 편하게 말씀해 주세요.",
	"es": "Hola, dime qué quieres que haga.",
	"mx": "Hola, dime qué quieres que haga.",
	"pt": "Olá, é só me dizer o que você quer que eu faça.",
	"fr": "Bonjour, dites-moi simplement ce que vous voulez que je fasse.",
	"de": "Hallo, sag mir einfach, was ich für dich tun soll.",
	"it": "Ciao, dimmi pure cosa vuoi che faccia.",
	"ru": "Привет, просто скажите, что мне сделать.",
	"id": "Halo, katakan saja apa yang ingin Anda saya kerjakan.",
	"ms": "Hai, beritahu sahaja apa yang anda mahu saya buat.",
	"th": "สวัสดีค่ะ อยากให้ฉันทำอะไร บอกได้เลยนะคะ",
	"vi": "Xin chào, bạn cứ nói tôi cần làm gì nhé.",
	"ar": "مرحبًا، أخبرني فقط بما تريدني أن أفعله.",
	"tl": "Kumusta, sabihin mo lang kung ano ang gusto mong gawin ko.",
}

// voiceSample is the line a voice says when tried: by the list's language
// for it, else by its id's prefix, else in the App's language.
func voiceSample(p *ttsProvider, speaker, uiLang string) string {
	lang := ""
	for _, v := range p.Voices {
		if v.ID == speaker {
			lang = v.Lang
			break
		}
	}
	if lang == "" {
		if i := strings.IndexByte(speaker, '_'); i > 0 {
			lang = speaker[:i]
		}
	}
	if s, ok := voiceSampleLines[lang]; ok {
		return s
	}
	if uiLang == "en" {
		return voiceSampleLines["en"]
	}
	return voiceSampleLines["zh"]
}

// previewVoice is POST /app/voice/preview {tts_provider, tts_speaker,
// tts_rate, tts_key?, lang?}: one short line in that voice, as a WAV, to
// try it before saving. The key is the one typed, else the one kept.
func (b *Backend) previewVoice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Provider string `json:"tts_provider"`
		Speaker  string `json:"tts_speaker"`
		Rate     int    `json:"tts_rate"`
		Key      string `json:"tts_key"`
		Lang     string `json:"lang"`
	}
	if !decode(w, r, &in) {
		return
	}
	p := ttsProviderOf(strings.TrimSpace(in.Provider))
	if p == nil {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "unknown speech service " + in.Provider,
			Suggestion: "GET /app/voice lists the speech services under tts_providers"})
		return
	}
	if in.Rate < -50 || in.Rate > 100 {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "speech rate must be between -50 and 100"})
		return
	}
	speaker := strings.TrimSpace(in.Speaker)
	if speaker == "" {
		speaker = p.DefaultSpeaker
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = b.ttsKey(p.ID)
	}
	if key == "" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "no_key", Message: "no API key for " + p.NameEn,
			Suggestion: "type the speech service's API key first"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var pcm bytes.Buffer
	rate := 0
	err := p.speech(key, VoiceConfig{TTSSpeaker: speaker, TTSRate: in.Rate}).Speak(ctx, voiceSample(p, speaker, in.Lang), func(b []byte, r int) {
		pcm.Write(b)
		rate = r
	})
	if err != nil {
		writeErr(w, &client.Error{Status: http.StatusBadGateway, Code: "tts_failed", Message: err.Error(), Retryable: true,
			Suggestion: "check the voice and the API key"})
		return
	}
	if pcm.Len() == 0 || rate == 0 {
		writeErr(w, &client.Error{Status: http.StatusBadGateway, Code: "tts_failed", Message: "the speech service sent no audio", Retryable: true})
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(wavPCM16(pcm.Bytes(), rate))
}

// wavPCM16 wraps 16-bit mono PCM in a WAV header.
func wavPCM16(pcm []byte, rate int) []byte {
	var h bytes.Buffer
	le := binary.LittleEndian
	h.WriteString("RIFF")
	_ = binary.Write(&h, le, uint32(36+len(pcm)))
	h.WriteString("WAVEfmt ")
	_ = binary.Write(&h, le, uint32(16))
	_ = binary.Write(&h, le, uint16(1)) // PCM
	_ = binary.Write(&h, le, uint16(1)) // mono
	_ = binary.Write(&h, le, uint32(rate))
	_ = binary.Write(&h, le, uint32(rate*2))
	_ = binary.Write(&h, le, uint16(2))
	_ = binary.Write(&h, le, uint16(16))
	h.WriteString("data")
	_ = binary.Write(&h, le, uint32(len(pcm)))
	h.Write(pcm)
	return h.Bytes()
}
