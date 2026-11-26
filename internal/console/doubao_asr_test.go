package console

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/myco-ai/myco/internal/oscap"
)

const testDoubaoKey = "sk-test-doubao-key-123"

func withDoubaoURLs(t *testing.T, flash, stream string) {
	t.Helper()
	of, os := doubaoFlashURL, doubaoStreamURL
	if flash != "" {
		doubaoFlashURL = flash
	}
	if stream != "" {
		doubaoStreamURL = stream
	}
	t.Cleanup(func() { doubaoFlashURL, doubaoStreamURL = of, os })
}

func TestDoubaoFlashRecognizes(t *testing.T) {
	var got struct {
		User    map[string]any `json:"user"`
		Audio   map[string]any `json:"audio"`
		Request map[string]any `json:"request"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != testDoubaoKey || r.Header.Get("X-Api-Resource-Id") != "volc.bigasr.auc_turbo" || r.Header.Get("X-Api-Sequence") != "-1" || r.Header.Get("X-Api-Request-Id") == "" {
			t.Errorf("headers: %v", r.Header)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("X-Api-Status-Code", "20000000")
		w.Header().Set("X-Api-Message", "OK")
		_, _ = io.WriteString(w, `{"audio_info":{"duration":4200},"result":{"text":"让 Claude 看一下这个 PR。推到 GitHub。","utterances":[
			{"text":"让 Claude 看一下这个 PR。","start_time":450,"end_time":2100},{"text":"推到 GitHub。","start_time":2300,"end_time":4100}]}}`)
	}))
	defer srv.Close()
	withDoubaoURLs(t, srv.URL, "")
	segs, err := doubaoFlash(context.Background(), srv.Client(), testDoubaoKey, []byte("RIFF-wav"), "", []string{"Claude", "GitHub", "Kettu"})
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 || segs[0].Text != "让 Claude 看一下这个 PR。" || segs[0].Start != 0.45 || segs[1].End != 4.1 {
		t.Fatalf("segments: %+v", segs)
	}
	if b, _ := base64.StdEncoding.DecodeString(got.Audio["data"].(string)); string(b) != "RIFF-wav" {
		t.Fatalf("audio: %v", got.Audio)
	}
	corpus, _ := got.Request["corpus"].(map[string]any)
	ctxJSON, _ := corpus["context"].(string)
	var hw struct {
		Hotwords []struct{ Word string } `json:"hotwords"`
	}
	if json.Unmarshal([]byte(ctxJSON), &hw) != nil || len(hw.Hotwords) != 3 || hw.Hotwords[0].Word != "Claude" {
		t.Fatalf("hot words: %q", ctxJSON)
	}
	if got.Request["model_name"] != "bigmodel" {
		t.Fatalf("request: %v", got.Request)
	}
}

func TestDoubaoFlashSilenceAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
		msg    string
		want   string
	}{
		{"silence", 200, "20000003", "silence", ""},
		{"not turned on", 403, "", `{"error":"requested resource not granted"}`, oscap.CodeServiceNotEnabled},
		{"not granted, as a code", 200, "45000030", "requested grant not found for resource volc.bigasr.auc_turbo", oscap.CodeServiceNotEnabled},
		{"bad key, echoed back", 401, "", "invalid X-Api-Key: " + testDoubaoKey, oscap.CodeNoCredentials},
		{"busy", 200, "55000031", "server busy", oscap.CodeFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.code != "" {
					w.Header().Set("X-Api-Status-Code", tc.code)
					w.Header().Set("X-Api-Message", tc.msg)
				}
				w.WriteHeader(tc.status)
				if tc.code == "" {
					_, _ = io.WriteString(w, tc.msg)
				}
			}))
			defer srv.Close()
			withDoubaoURLs(t, srv.URL, "")
			segs, err := doubaoFlash(context.Background(), srv.Client(), testDoubaoKey, []byte("x"), "", nil)
			if tc.want == "" {
				if err != nil || len(segs) != 0 {
					t.Fatalf("silence: %v %+v", err, segs)
				}
				return
			}
			var oe *oscap.Error
			if !errors.As(err, &oe) || oe.Code != tc.want {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if strings.Contains(oe.Message, testDoubaoKey) {
				t.Fatalf("the key is in the error: %s", oe.Message)
			}
			if tc.want == oscap.CodeServiceNotEnabled && !strings.Contains(oe.Message, "极速版") {
				t.Fatalf("should say which service to turn on: %s", oe.Message)
			}
		})
	}
	if _, err := doubaoFlash(context.Background(), nil, "", []byte("x"), "", nil); err == nil || !strings.Contains(err.Error(), "Settings › Voice") {
		t.Fatalf("no key: %v", err)
	}
}

// A fake streaming server: checks the requests, answers as it hears.
func TestDoubaoStream(t *testing.T) {
	var mu sync.Mutex
	var audio int
	var first map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != testDoubaoKey || r.Header.Get("X-Api-Resource-Id") != doubaoStreamResource {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		seq := uint32(0)
		answer := func(text string, last bool) {
			seq++
			flags := byte(dbFlagSeq)
			if last {
				flags |= dbFlagLast
			}
			payload, _ := json.Marshal(map[string]any{"result": map[string]any{"text": text}})
			b := dbHeader(dbFullResponse, flags, dbJSON)
			b = binary.BigEndian.AppendUint32(b, seq)
			b = binary.BigEndian.AppendUint32(b, uint32(len(payload)))
			_ = c.Write(ctx, websocket.MessageBinary, append(b, payload...))
		}
		for {
			_, f, err := c.Read(ctx)
			if err != nil {
				return
			}
			typ, flags := f[1]>>4, f[1]&0x0f
			n := binary.BigEndian.Uint32(f[4:])
			payload := f[8 : 8+n]
			switch typ {
			case dbFullRequest:
				mu.Lock()
				_ = json.Unmarshal(payload, &first)
				mu.Unlock()
			case dbAudioOnly:
				mu.Lock()
				audio += len(payload)
				mu.Unlock()
				if flags&dbFlagLast != 0 {
					answer("让 Claude 看一下", true)
					return
				}
				answer("让 Claude", false)
			}
		}
	}))
	defer srv.Close()
	withDoubaoURLs(t, "", "ws"+strings.TrimPrefix(srv.URL, "http"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var heard []string
	var final bool
	var hmu sync.Mutex
	s, err := dialDoubaoStream(ctx, testDoubaoKey, []string{"Claude"}, func(text string, f bool) {
		hmu.Lock()
		heard, final = append(heard, text), f
		hmu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.Send(ctx, make([]byte, 6400)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	hmu.Lock()
	defer hmu.Unlock()
	if len(heard) == 0 || heard[len(heard)-1] != "让 Claude 看一下" || !final {
		t.Fatalf("heard: %v final=%v", heard, final)
	}
	mu.Lock()
	defer mu.Unlock()
	if audio != 3*6400 {
		t.Fatalf("audio: %d", audio)
	}
	req, _ := first["request"].(map[string]any)
	corpus, _ := req["corpus"].(map[string]any)
	if a, _ := first["audio"].(map[string]any); a["rate"] != float64(16000) || !strings.Contains(corpus["context"].(string), "Claude") {
		t.Fatalf("first request: %v", first)
	}
}

func TestDoubaoStreamErrorFrame(t *testing.T) {
	msg := []byte("requested resource not granted")
	b := dbHeader(dbError, 0, dbJSON)
	b = binary.BigEndian.AppendUint32(b, 45000030)
	b = binary.BigEndian.AppendUint32(b, uint32(len(msg)))
	_, last, err := dbParse(append(b, msg...))
	var se *doubaoStreamErr
	if !last || !errors.As(err, &se) || se.code != 45000030 {
		t.Fatalf("error frame: %v %v", last, err)
	}
	var oe *oscap.Error
	if e := doubaoError(doubaoStreamService, 0, se.code, se.msg, testDoubaoKey); !errors.As(e, &oe) || oe.Code != oscap.CodeServiceNotEnabled {
		t.Fatalf("classified: %v", e)
	}
}

func TestAppCloudSpeechNeedsAKey(t *testing.T) {
	b := &Backend{Secrets: &memKeys{m: map[string]string{}}}
	if code, why := (appCloudSpeech{b}).Status("doubao"); code != oscap.CodeNoCredentials || !strings.Contains(why, "Settings › Voice") {
		t.Fatalf("no key: %s %s", code, why)
	}
	_ = b.Secrets.Set(ttsSecret("doubao"), testDoubaoKey)
	if code, _ := (appCloudSpeech{b}).Status("doubao"); code != "" {
		t.Fatalf("with a key: %s", code)
	}
	if code, _ := (appCloudSpeech{b}).Status("whisper"); code != oscap.CodeUnsupported {
		t.Fatalf("other engine: %s", code)
	}
}
