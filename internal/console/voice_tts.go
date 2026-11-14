package console

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// voiceSpeech says text: audio goes to play as it arrives (16-bit mono PCM).
type voiceSpeech interface {
	Speak(ctx context.Context, text string, play func(pcm []byte, rate int)) error
}

// doubaoTTS is Doubao (Volcengine) speech synthesis over its streaming HTTP
// API, with the key from the App's settings (an API key from the Volcengine
// console). A cloned voice (S_...) uses the seed-icl-2.0 resource, the
// others seed-tts-2.0.
type doubaoTTS struct {
	key, speaker string
	rate         int
	http         *http.Client
}

var doubaoTTSURL = "https://openspeech.bytedance.com/api/v3/tts/unidirectional"

const doubaoRate = 24000

func (d *doubaoTTS) Speak(ctx context.Context, text string, play func([]byte, int)) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	resource := "seed-tts-2.0"
	if strings.HasPrefix(d.speaker, "S_") {
		resource = "seed-icl-2.0"
	}
	body, _ := json.Marshal(map[string]any{"req_params": map[string]any{
		"text": text, "speaker": d.speaker,
		"audio_params": map[string]any{"format": "pcm", "sample_rate": doubaoRate, "speech_rate": d.rate},
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, doubaoTTSURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", d.key)
	req.Header.Set("X-Api-Resource-Id", resource)
	req.Header.Set("X-Api-Request-Id", requestID())
	hc := d.http
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		return fmt.Errorf("豆包语音 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return readDoubaoStream(resp.Body, play)
}

// readDoubaoStream: one JSON object per line; data is base64 audio; code
// 20000000 ends it. Odd bytes wait for the next chunk (samples are 2 bytes).
func readDoubaoStream(r io.Reader, play func([]byte, int)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	var carry []byte
	finished := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var f struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    string `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			continue
		}
		if f.Code == 20000000 {
			finished = true
			break
		}
		if f.Code != 0 {
			return fmt.Errorf("豆包语音 %d: %s", f.Code, f.Message)
		}
		if f.Data == "" {
			continue
		}
		pcm, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil {
			continue
		}
		pcm = append(carry, pcm...)
		carry = nil
		if len(pcm)%2 == 1 {
			carry = []byte{pcm[len(pcm)-1]}
			pcm = pcm[:len(pcm)-1]
		}
		if len(pcm) > 0 {
			play(pcm, doubaoRate)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if !finished {
		return fmt.Errorf("豆包语音没有正常结束")
	}
	return nil
}

func requestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
