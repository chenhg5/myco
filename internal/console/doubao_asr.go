package console

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/myco-ai/myco/internal/oscap"
)

// Speech recognition by Doubao (Volcengine's 豆包大模型语音识别), with the key
// saved for Doubao speech synthesis in Settings › Voice (one Volcengine API
// key, X-Api-Key, serves both once the account has turned recognition on).
// It hears Chinese with English mixed in, and is told the words to listen
// for (speech_words.go) as hot words. The key stays in this process: it is
// never logged, printed or put in an error.
//
//   - doubaoFlash: a recording, in one request (录音文件识别极速版), for
//     speech.transcribe's engine "doubao";
//   - doubaoStream: speech as it is said (流式语音识别, a websocket), for
//     dictation and calls later.

var (
	doubaoFlashURL  = "https://openspeech.bytedance.com/api/v3/auc/bigmodel/recognize/flash"
	doubaoStreamURL = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"
)

const (
	doubaoFlashResource = "volc.bigasr.auc_turbo"
	// The streaming model 2.0, then 1.0 for accounts that have only that.
	doubaoStreamResource   = "volc.seedasr.sauc.duration"
	doubaoStreamResourceV1 = "volc.bigasr.sauc.duration"
	doubaoOK               = 20000000
	doubaoSilence          = 20000003
)

// The services to turn on, as the Volcengine console names them.
const (
	doubaoFlashService  = "豆包录音文件识别（极速版）"
	doubaoStreamService = "豆包流式语音识别"
)

// doubaoHotwords is the request's context: the words to listen for.
func doubaoHotwords(words []string) string {
	type hw struct {
		Word string `json:"word"`
	}
	list := make([]hw, 0, len(words))
	for _, w := range words {
		if w = strings.TrimSpace(w); w != "" {
			list = append(list, hw{w})
		}
	}
	b, _ := json.Marshal(map[string]any{"hotwords": list})
	return string(b)
}

// doubaoError is what went wrong, as speech.transcribe's codes.
func doubaoError(service string, httpStatus int, code int, msg, key string) error {
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "…") // never the key, even echoed back
	}
	msg = clipRunes(strings.TrimSpace(msg), 300)
	low := strings.ToLower(msg)
	switch {
	case httpStatus == http.StatusUnauthorized || strings.Contains(low, "invalid api key") || strings.Contains(low, "invalid x-api-key") || strings.Contains(low, "unauthorized"):
		return &oscap.Error{Code: oscap.CodeNoCredentials, Message: "the Doubao speech key was not accepted (Settings › Voice): " + msg}
	case httpStatus == http.StatusForbidden || strings.Contains(low, "not granted") || strings.Contains(low, "resource") && (strings.Contains(low, "grant") || strings.Contains(low, "permission") || strings.Contains(low, "not found")) ||
		strings.Contains(msg, "未开通") || strings.Contains(msg, "未授权") || strings.Contains(low, "requested resource"):
		return &oscap.Error{Code: oscap.CodeServiceNotEnabled, Message: "turn on " + service + " for this API key in the Volcengine console (豆包语音 › 开通服务): " + msg}
	case code == 55000031 || httpStatus == http.StatusTooManyRequests:
		return &oscap.Error{Code: oscap.CodeFailed, Message: "Doubao speech recognition is busy; try again: " + msg}
	}
	if code != 0 {
		return &oscap.Error{Code: oscap.CodeFailed, Message: fmt.Sprintf("Doubao speech recognition %d: %s", code, msg)}
	}
	return &oscap.Error{Code: oscap.CodeFailed, Message: fmt.Sprintf("Doubao speech recognition HTTP %d: %s", httpStatus, msg)}
}

// doubaoFlash recognizes a recording (WAV, MP3 or OGG Opus) in one request.
func doubaoFlash(ctx context.Context, hc *http.Client, key string, audio []byte, lang string, hotwords []string) ([]oscap.Segment, error) {
	if key == "" {
		return nil, &oscap.Error{Code: oscap.CodeNoCredentials, Message: "add the Doubao speech API key under Settings › Voice"}
	}
	request := map[string]any{
		"model_name": "bigmodel", "enable_itn": true, "enable_punc": true, "show_utterances": true,
	}
	if len(hotwords) > 0 {
		request["corpus"] = map[string]any{"context": doubaoHotwords(hotwords)}
	}
	audioPart := map[string]any{"data": base64.StdEncoding.EncodeToString(audio)}
	if lang != "" {
		audioPart["language"] = lang
	}
	body, _ := json.Marshal(map[string]any{"user": map[string]any{"uid": "myco"}, "audio": audioPart, "request": request})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, doubaoFlashURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", key)
	req.Header.Set("X-Api-Resource-Id", doubaoFlashResource)
	req.Header.Set("X-Api-Request-Id", requestID())
	req.Header.Set("X-Api-Sequence", "-1")
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	code, _ := strconv.Atoi(resp.Header.Get("X-Api-Status-Code"))
	msg := resp.Header.Get("X-Api-Message")
	if resp.StatusCode/100 != 2 {
		if msg == "" {
			msg = string(raw)
		}
		return nil, doubaoError(doubaoFlashService, resp.StatusCode, code, msg, key)
	}
	switch code {
	case doubaoOK, 0:
	case doubaoSilence:
		return []oscap.Segment{}, nil
	default:
		if msg == "" {
			msg = string(raw)
		}
		return nil, doubaoError(doubaoFlashService, resp.StatusCode, code, msg, key)
	}
	var out struct {
		AudioInfo struct {
			Duration float64 `json:"duration"`
		} `json:"audio_info"`
		Result doubaoResult `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("Doubao speech recognition: unreadable answer: %w", err)
	}
	return out.Result.segments(out.AudioInfo.Duration), nil
}

// doubaoResult is a recognition result: the text, and its sentences with
// times in milliseconds.
type doubaoResult struct {
	Text       string `json:"text"`
	Utterances []struct {
		Text      string  `json:"text"`
		StartTime float64 `json:"start_time"`
		EndTime   float64 `json:"end_time"`
		Definite  bool    `json:"definite"`
	} `json:"utterances"`
}

func (r doubaoResult) segments(durationMS float64) []oscap.Segment {
	segs := []oscap.Segment{}
	for _, u := range r.Utterances {
		if t := strings.TrimSpace(u.Text); t != "" {
			segs = append(segs, oscap.Segment{Start: u.StartTime / 1000, End: u.EndTime / 1000, Text: t})
		}
	}
	if len(segs) == 0 && strings.TrimSpace(r.Text) != "" {
		segs = append(segs, oscap.Segment{Start: 0, End: durationMS / 1000, Text: strings.TrimSpace(r.Text)})
	}
	return segs
}

// --- streaming (the binary websocket protocol) ---

// Message types and flags of the protocol's 4-byte header.
const (
	dbFullRequest  = 0b0001
	dbAudioOnly    = 0b0010
	dbFullResponse = 0b1001
	dbError        = 0b1111
	dbFlagSeq      = 0b0001 // a sequence number follows the header
	dbFlagLast     = 0b0010 // the last packet
	dbJSON         = 0b0001
)

func dbHeader(msgType, flags, serialization byte) []byte {
	return []byte{0b0001<<4 | 0b0001, msgType<<4 | flags, serialization << 4, 0}
}

func dbFrame(msgType, flags, serialization byte, payload []byte) []byte {
	b := dbHeader(msgType, flags, serialization)
	b = binary.BigEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}

// dbParse reads a server frame: a result (JSON), or an error.
func dbParse(frame []byte) (result *doubaoResult, last bool, err error) {
	if len(frame) < 4 {
		return nil, false, errors.New("short frame")
	}
	hs := int(frame[0]&0x0f) * 4
	msgType, flags := frame[1]>>4, frame[1]&0x0f
	if hs < 4 || len(frame) < hs {
		return nil, false, errors.New("bad header")
	}
	rest := frame[hs:]
	switch msgType {
	case dbError:
		if len(rest) < 8 {
			return nil, true, errors.New("short error frame")
		}
		code := binary.BigEndian.Uint32(rest)
		n := binary.BigEndian.Uint32(rest[4:])
		msg := rest[8:]
		if int(n) <= len(msg) {
			msg = msg[:n]
		}
		return nil, true, &doubaoStreamErr{code: int(code), msg: string(msg)}
	case dbFullResponse:
		if flags&dbFlagSeq != 0 {
			if len(rest) < 4 {
				return nil, false, errors.New("short frame")
			}
			rest = rest[4:]
		}
		if len(rest) < 4 {
			return nil, false, errors.New("short frame")
		}
		n := binary.BigEndian.Uint32(rest)
		payload := rest[4:]
		if int(n) <= len(payload) {
			payload = payload[:n]
		}
		var r struct {
			Result doubaoResult `json:"result"`
		}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &r); err != nil {
				return nil, false, err
			}
		}
		return &r.Result, flags&dbFlagLast != 0, nil
	}
	return nil, false, nil
}

type doubaoStreamErr struct {
	code int
	msg  string
}

func (e *doubaoStreamErr) Error() string { return fmt.Sprintf("%d: %s", e.code, e.msg) }

// doubaoStream is one streaming recognition: Send audio (16 kHz mono 16-bit
// PCM, about 200 ms at a time), then Close; what it hears so far comes to
// heard (the whole text, final at the end).
type doubaoStream struct {
	conn *websocket.Conn
	key  string
	done chan error
}

func dialDoubaoStream(ctx context.Context, key string, hotwords []string, heard func(text string, final bool)) (*doubaoStream, error) {
	if key == "" {
		return nil, &oscap.Error{Code: oscap.CodeNoCredentials, Message: "add the Doubao speech API key under Settings › Voice"}
	}
	var conn *websocket.Conn
	var lastErr error
	for _, resource := range []string{doubaoStreamResource, doubaoStreamResourceV1} {
		h := http.Header{}
		h.Set("X-Api-Key", key)
		h.Set("X-Api-Resource-Id", resource)
		h.Set("X-Api-Connect-Id", requestID())
		c, resp, err := websocket.Dial(ctx, doubaoStreamURL, &websocket.DialOptions{HTTPHeader: h})
		if err == nil {
			conn = c
			break
		}
		status, msg := 0, err.Error()
		if resp != nil {
			status = resp.StatusCode
			if b, _ := io.ReadAll(io.LimitReader(resp.Body, 2000)); len(b) > 0 {
				msg = string(b)
			}
		}
		lastErr = doubaoError(doubaoStreamService, status, 0, msg, key)
		var oe *oscap.Error
		if !errors.As(lastErr, &oe) || oe.Code != oscap.CodeServiceNotEnabled {
			return nil, lastErr // only a resource not on: the older model next
		}
	}
	if conn == nil {
		return nil, lastErr
	}
	conn.SetReadLimit(4 << 20)
	request := map[string]any{"model_name": "bigmodel", "enable_itn": true, "enable_punc": true, "result_type": "full", "show_utterances": true}
	if len(hotwords) > 0 {
		request["corpus"] = map[string]any{"context": doubaoHotwords(hotwords)}
	}
	first, _ := json.Marshal(map[string]any{
		"user":    map[string]any{"uid": "myco"},
		"audio":   map[string]any{"format": "pcm", "codec": "raw", "rate": 16000, "bits": 16, "channel": 1},
		"request": request,
	})
	if err := conn.Write(ctx, websocket.MessageBinary, dbFrame(dbFullRequest, 0, dbJSON, first)); err != nil {
		conn.CloseNow()
		return nil, err
	}
	s := &doubaoStream{conn: conn, key: key, done: make(chan error, 1)}
	go func() {
		for {
			_, frame, err := conn.Read(ctx)
			if err != nil {
				s.done <- err
				return
			}
			r, last, err := dbParse(frame)
			if err != nil {
				var se *doubaoStreamErr
				if errors.As(err, &se) {
					err = doubaoError(doubaoStreamService, 0, se.code, se.msg, key)
				}
				s.done <- err
				return
			}
			if r != nil && heard != nil {
				heard(strings.TrimSpace(r.Text), last)
			}
			if last {
				s.done <- nil
				return
			}
		}
	}()
	return s, nil
}

// Send passes a piece of audio on.
func (s *doubaoStream) Send(ctx context.Context, pcm []byte) error {
	return s.conn.Write(ctx, websocket.MessageBinary, dbFrame(dbAudioOnly, 0, 0, pcm))
}

// Close says the audio has ended and waits for the last words.
func (s *doubaoStream) Close(ctx context.Context) error {
	defer s.conn.CloseNow()
	if err := s.conn.Write(ctx, websocket.MessageBinary, dbFrame(dbAudioOnly, dbFlagLast, 0, nil)); err != nil {
		return err
	}
	select {
	case err := <-s.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- the App's cloud speech for capabilities (oscap.CloudSpeech) ---

type appCloudSpeech struct{ b *Backend }

func (c appCloudSpeech) Status(engine string) (string, string) {
	if engine != "doubao" {
		return oscap.CodeUnsupported, "no such engine: " + engine
	}
	if c.b.ttsKey("doubao") == "" {
		return oscap.CodeNoCredentials, "add the Doubao speech API key under Settings › Voice, and turn on " + doubaoFlashService + " for it in the Volcengine console"
	}
	return "", ""
}

func (c appCloudSpeech) Transcribe(ctx context.Context, engine string, wav []byte, lang string) ([]oscap.Segment, error) {
	if code, why := c.Status(engine); code != "" {
		return nil, &oscap.Error{Code: code, Message: why}
	}
	words, _ := c.b.speechVocab(ctx)
	return doubaoFlash(ctx, nil, c.b.ttsKey("doubao"), wav, lang, words)
}
