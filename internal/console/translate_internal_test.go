package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type mapSecrets map[string]string

func (m mapSecrets) Get(id string) (string, error) {
	if v, ok := m[id]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}
func (m mapSecrets) Set(id, v string) error { m[id] = v; return nil }
func (m mapSecrets) Delete(id string) error { delete(m, id); return nil }

func translateBackend(t *testing.T) (*Backend, http.Handler, mapSecrets) {
	t.Helper()
	sec := mapSecrets{}
	b := &Backend{Store: NewStore(filepath.Join(t.TempDir(), "console-hubs.json")), Secrets: sec}
	return b, b.Handler(), sec
}

func call(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestBlocksKeepCode(t *testing.T) {
	parts, code := blocks("Hello there.\n\n```go\nfmt.Println(1)\n\nx := 2\n```\n\nBye.")
	if len(parts) != 3 || code[0] || !code[1] || code[2] || !strings.Contains(parts[1], "x := 2") {
		t.Fatalf("parts %q code %v", parts, code)
	}
}

func TestPiecesCutAtSentences(t *testing.T) {
	s := strings.Repeat("一句话。", 60) // 720 bytes
	ps := pieces(s, 450)
	if len(ps) != 2 || strings.Join(ps, "") != s {
		t.Fatalf("pieces: %d %q", len(ps), ps)
	}
	for _, p := range ps {
		if len(p) > 450 || !strings.HasSuffix(p, "。") {
			t.Fatalf("piece %q", p)
		}
	}
}

func TestTranslateMyMemoryByDefault(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("langpair") != "en|zh-CN" {
			t.Errorf("langpair %q", r.URL.Query().Get("langpair"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"responseData": map[string]any{"translatedText": "【" + r.URL.Query().Get("q") + "】"}, "responseStatus": 200})
	}))
	defer srv.Close()
	old := translateEndpoints.mymemory
	translateEndpoints.mymemory = srv.URL
	defer func() { translateEndpoints.mymemory = old }()

	_, h, _ := translateBackend(t)
	code, out := call(t, h, "POST", "/app/translate", `{"text":"Tests pass.\n\n`+"```\\nmake check\\n```"+`\n\nShip it.","to":"zh"}`)
	if code != 200 || out["provider"] != "mymemory" {
		t.Fatalf("%d %v", code, out)
	}
	want := "【Tests pass.】\n\n```\nmake check\n```\n\n【Ship it.】"
	if out["text"] != want {
		t.Fatalf("got %q", out["text"])
	}
	// The same again comes from the cache.
	n := calls.Load()
	call(t, h, "POST", "/app/translate", `{"text":"Tests pass.\n\n`+"```\\nmake check\\n```"+`\n\nShip it.","to":"zh"}`)
	if calls.Load() != n {
		t.Fatal("a second translation of the same text should be cached")
	}
}

func TestTranslateMyMemoryQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"responseData": map[string]any{"translatedText": "MYMEMORY WARNING: YOU USED ALL AVAILABLE FREE TRANSLATIONS FOR TODAY"}, "responseStatus": 429, "quotaFinished": true})
	}))
	defer srv.Close()
	old := translateEndpoints.mymemory
	translateEndpoints.mymemory = srv.URL
	defer func() { translateEndpoints.mymemory = old }()
	_, h, _ := translateBackend(t)
	code, out := call(t, h, "POST", "/app/translate", `{"text":"hello","to":"zh"}`)
	if code != http.StatusTooManyRequests || out["error"] != "quota" {
		t.Fatalf("%d %v", code, out)
	}
}

func TestTranslateDeepLKeyStaysInKeychain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "DeepL-Auth-Key k-123:fx" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var in struct {
			Text   []string `json:"text"`
			Target string   `json:"target_lang"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		tr := []map[string]string{}
		for _, s := range in.Text {
			tr = append(tr, map[string]string{"text": in.Target + ":" + s})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"translations": tr})
	}))
	defer srv.Close()
	oldF := translateEndpoints.deeplFree
	translateEndpoints.deeplFree = srv.URL
	defer func() { translateEndpoints.deeplFree = oldF }()

	b, h, sec := translateBackend(t)
	// No key yet: say so.
	call(t, h, "PUT", "/app/translate/config", `{"provider":"deepl"}`)
	if code, out := call(t, h, "POST", "/app/translate", `{"text":"你好","to":"en"}`); code != http.StatusConflict || out["error"] != "not_configured" {
		t.Fatalf("without a key: %d %v", code, out)
	}
	code, out := call(t, h, "PUT", "/app/translate/config", `{"provider":"deepl","key":"k-123:fx"}`)
	if code != 200 || sec["translate:deepl"] != "k-123:fx" {
		t.Fatalf("%d %v %v", code, out, sec)
	}
	if keys, _ := out["keys"].(map[string]any); keys["deepl"] != true {
		t.Fatalf("keys %v", out["keys"])
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "k-123") {
		t.Fatal("the key must never be sent back")
	}
	if file, _ := filepath.Glob(filepath.Join(filepath.Dir(b.Store.path), "*")); len(file) > 0 {
		for _, f := range file {
			if data, _ := readFile(f); strings.Contains(data, "k-123") {
				t.Fatalf("the key must not be in %s", f)
			}
		}
	}
	code, out = call(t, h, "POST", "/app/translate", `{"text":"你好\n\n再见","to":"en"}`)
	if code != 200 || out["text"] != "EN-US:你好\n\nEN-US:再见" {
		t.Fatalf("%d %v", code, out)
	}
}

func TestTranslateOpenAICompatible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-x" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Model != "deepseek-chat" || !strings.Contains(in.Messages[0].Content, "Simplified Chinese") {
			t.Errorf("request %+v", in)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": " 译文 "}}}})
	}))
	defer srv.Close()
	_, h, _ := translateBackend(t)
	call(t, h, "PUT", "/app/translate/config", `{"provider":"openai","base_url":"`+srv.URL+`/v1/","model":"deepseek-chat","key":"sk-x"}`)
	code, out := call(t, h, "POST", "/app/translate", `{"text":"Done.","to":"zh"}`)
	if code != 200 || out["text"] != "译文" || out["provider"] != "openai" {
		t.Fatalf("%d %v", code, out)
	}
	// A wrong key is said as such.
	call(t, h, "PUT", "/app/translate/config", `{"provider":"openai","base_url":"`+srv.URL+`/v1","model":"deepseek-chat","key":"sk-wrong"}`)
	if code, out := call(t, h, "POST", "/app/translate", `{"text":"Done again.","to":"zh"}`); code != http.StatusUnauthorized || out["error"] != "unauthorized" {
		t.Fatalf("%d %v", code, out)
	}
}

func TestTranslateRejectsBadInput(t *testing.T) {
	_, h, _ := translateBackend(t)
	if code, _ := call(t, h, "POST", "/app/translate", `{"text":"hi","to":"fr"}`); code != http.StatusBadRequest {
		t.Fatalf("to=fr: %d", code)
	}
	if code, _ := call(t, h, "PUT", "/app/translate/config", `{"provider":"babel"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown provider: %d", code)
	}
	if code, _ := call(t, h, "PUT", "/app/translate/config", `{"provider":"openai","base_url":"file:///etc"}`); code != http.StatusBadRequest {
		t.Fatalf("bad base url: %d", code)
	}
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func TestPickImageOnlyPictures(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "face.png")
	_ = os.WriteFile(png, []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), 0o600)
	txt := filepath.Join(dir, "notes.png") // a picture's name, not a picture
	_ = os.WriteFile(txt, []byte("just text"), 0o600)
	b, _, _ := translateBackend(t)
	h := b.Handler()
	if code, _ := call(t, h, "POST", "/app/pick-image", ""); code != http.StatusNotFound {
		t.Fatalf("without a panel: %d", code)
	}
	var next string
	b.PickImage = func() (string, error) { return next, nil }
	h = b.Handler()
	next = ""
	if _, out := call(t, h, "POST", "/app/pick-image", ""); out["cancelled"] != true {
		t.Fatalf("cancelled: %v", out)
	}
	next = png
	if code, out := call(t, h, "POST", "/app/pick-image", ""); code != 200 || out["type"] != "image/png" || out["name"] != "face.png" || out["data"] == "" {
		t.Fatalf("picture: %d %v", code, out)
	}
	next = txt
	if code, out := call(t, h, "POST", "/app/pick-image", ""); code != http.StatusUnsupportedMediaType || out["error"] != "not_image" {
		t.Fatalf("not a picture: %d %v", code, out)
	}
}
