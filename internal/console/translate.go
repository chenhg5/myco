package console

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/client"
)

// Translating what agents write into the language the person reads, with
// a service they choose (the App's settings):
//
//   - mymemory: MyMemory's free public API, no account (about 5,000
//     characters a day without one); the default;
//   - deepl: DeepL with the person's key (a free key ends in ":fx");
//   - google: Google Cloud Translation with an API key;
//   - openai: any OpenAI-compatible chat API (OpenAI, DeepSeek,
//     OpenRouter, ...) with a key, a base URL and a model.
//
// Keys are kept in the system keychain (Secrets), never in the settings
// file, never sent back to the page, never logged. Code blocks are left as
// they are. Translations are cached in memory.
//
// GET /app/translate/config → the settings, and which services have a key.
// PUT /app/translate/config {provider, base_url, model, key?, clear_key?}.
// POST /app/translate {text, to: "zh"|"en"} → {text, provider}.

// TranslateConfig is what the person chose (no keys).
type TranslateConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url,omitempty"`
	Model    string `json:"model,omitempty"`
}

var translateProviders = map[string]bool{"mymemory": true, "deepl": true, "google": true, "openai": true}

// Where the services are (tests point them elsewhere).
var translateEndpoints = struct {
	mymemory, deeplFree, deeplPro, google string
}{
	mymemory:  "https://api.mymemory.translated.net/get",
	deeplFree: "https://api-free.deepl.com/v2/translate",
	deeplPro:  "https://api.deepl.com/v2/translate",
	google:    "https://translation.googleapis.com/language/translate/v2",
}

type translator struct {
	mu    sync.Mutex
	path  string
	cache map[string]string
	http  *http.Client
}

func secretName(provider string) string { return "translate:" + provider }

func (b *Backend) translatorState() *translator {
	b.translateOnce.Do(func() {
		path := ""
		if b.Store != nil {
			path = filepath.Join(filepath.Dir(b.Store.path), "console-translate.json")
		}
		b.translate = &translator{path: path, cache: map[string]string{}, http: &http.Client{Timeout: 25 * time.Second}}
	})
	return b.translate
}

func (t *translator) config() TranslateConfig {
	c := TranslateConfig{Provider: "mymemory"}
	if t.path == "" {
		return c
	}
	if raw, err := os.ReadFile(t.path); err == nil {
		_ = json.Unmarshal(raw, &c)
	}
	if !translateProviders[c.Provider] {
		c.Provider = "mymemory"
	}
	return c
}

func (t *translator) save(c TranslateConfig) error {
	if t.path == "" {
		return errors.New("no place to keep the settings")
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0o700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(t.path, raw, 0o600)
}

func (b *Backend) translateRoutes(m *http.ServeMux) {
	keyed := func() map[string]bool {
		out := map[string]bool{}
		for p := range translateProviders {
			if p == "mymemory" || b.Secrets == nil {
				continue
			}
			if k, err := b.Secrets.Get(secretName(p)); err == nil && k != "" {
				out[p] = true
			}
		}
		return out
	}
	m.HandleFunc("GET /app/translate/config", func(w http.ResponseWriter, r *http.Request) {
		c := b.translatorState().config()
		writeJSON(w, http.StatusOK, map[string]any{"provider": c.Provider, "base_url": c.BaseURL, "model": c.Model, "keys": keyed()})
	})
	m.HandleFunc("PUT /app/translate/config", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			TranslateConfig
			Key      string `json:"key"`
			ClearKey bool   `json:"clear_key"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !translateProviders[in.Provider] {
			writeErr(w, &client.Error{Code: "invalid_argument", Message: "unknown translation service", Status: http.StatusBadRequest})
			return
		}
		if in.BaseURL != "" {
			if u, err := url.Parse(in.BaseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				writeErr(w, &client.Error{Code: "invalid_argument", Message: "the base URL must be an http(s) address", Status: http.StatusBadRequest})
				return
			}
		}
		t := b.translatorState()
		if err := t.save(in.TranslateConfig); err != nil {
			writeErr(w, err)
			return
		}
		if in.Provider != "mymemory" && b.Secrets != nil {
			var err error
			switch {
			case in.ClearKey:
				err = b.Secrets.Delete(secretName(in.Provider))
			case strings.TrimSpace(in.Key) != "":
				err = b.Secrets.Set(secretName(in.Provider), strings.TrimSpace(in.Key))
			}
			if err != nil {
				writeErr(w, &client.Error{Code: "unavailable", Message: "could not keep the key in the system keychain", Status: http.StatusConflict})
				return
			}
		}
		t.mu.Lock()
		t.cache = map[string]string{} // another service: translate afresh
		t.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"provider": in.Provider, "base_url": in.BaseURL, "model": in.Model, "keys": keyed()})
	})
	m.HandleFunc("POST /app/translate", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text string `json:"text"`
			To   string `json:"to"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.To != "zh" && in.To != "en" {
			writeErr(w, &client.Error{Code: "invalid_argument", Message: "to must be zh or en", Status: http.StatusBadRequest})
			return
		}
		if strings.TrimSpace(in.Text) == "" || len(in.Text) > 20000 {
			writeErr(w, &client.Error{Code: "invalid_argument", Message: "nothing to translate, or too long (20,000 bytes at most)", Status: http.StatusBadRequest})
			return
		}
		t := b.translatorState()
		c := t.config()
		key := ""
		if c.Provider != "mymemory" {
			if b.Secrets != nil {
				key, _ = b.Secrets.Get(secretName(c.Provider))
			}
			if key == "" {
				writeErr(w, &client.Error{Code: "not_configured", Message: "this translation service needs a key (Settings)", Status: http.StatusConflict})
				return
			}
		}
		sum := sha256.Sum256([]byte(c.Provider + "\x00" + c.BaseURL + "\x00" + c.Model + "\x00" + in.To + "\x00" + in.Text))
		ck := hex.EncodeToString(sum[:])
		t.mu.Lock()
		hit, ok := t.cache[ck]
		t.mu.Unlock()
		if ok {
			writeJSON(w, http.StatusOK, map[string]string{"text": hit, "provider": c.Provider})
			return
		}
		out, err := t.translate(r.Context(), c, key, in.Text, in.To)
		if err != nil {
			writeErr(w, err)
			return
		}
		t.mu.Lock()
		if len(t.cache) > 2000 {
			t.cache = map[string]string{}
		}
		t.cache[ck] = out
		t.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]string{"text": out, "provider": c.Provider})
	})
}

// translateErr is a failure the page can say in words: the service's own
// message is not passed on (it may echo the key or the text).
func translateErr(code, msg string) error {
	status := http.StatusBadGateway
	switch code {
	case "quota":
		status = http.StatusTooManyRequests
	case "unauthorized":
		status = http.StatusUnauthorized
	}
	return &client.Error{Code: code, Message: msg, Status: status}
}

var fence = regexp.MustCompile("^\\s*(```|~~~)")

// blocks splits Markdown into paragraphs, with code blocks marked: those
// are kept as they are.
func blocks(text string) (parts []string, code []bool) {
	var cur []string
	inFence := false
	flush := func(isCode bool) {
		if len(cur) > 0 {
			parts = append(parts, strings.Join(cur, "\n"))
			code = append(code, isCode)
			cur = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if fence.MatchString(line) {
			if !inFence {
				flush(false)
				inFence = true
				cur = append(cur, line)
				continue
			}
			cur = append(cur, line)
			flush(true)
			inFence = false
			continue
		}
		if inFence {
			cur = append(cur, line)
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush(false)
			continue
		}
		cur = append(cur, line)
	}
	flush(inFence)
	return parts, code
}

// pieces cuts a paragraph into parts of at most max bytes, at the end of a
// sentence or a line where it can.
func pieces(s string, max int) []string {
	var out []string
	for len(s) > max {
		cut := -1
		for i, r := range s {
			if i > max {
				break
			}
			if strings.ContainsRune(".!?。！？；;\n", r) {
				cut = i + len(string(r))
			}
		}
		if cut <= 0 {
			cut = max
			for cut > 0 && (s[cut]&0xC0) == 0x80 { // not inside a character
				cut--
			}
		}
		out = append(out, s[:cut])
		s = strings.TrimLeft(s[cut:], " ")
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

func (t *translator) translate(ctx context.Context, c TranslateConfig, key, text, to string) (string, error) {
	if c.Provider == "openai" {
		return t.openAI(ctx, c, key, text, to)
	}
	parts, code := blocks(text)
	var todo []string
	for i, p := range parts {
		if !code[i] {
			todo = append(todo, p)
		}
	}
	var done []string
	var err error
	switch c.Provider {
	case "deepl":
		done, err = t.deepL(ctx, key, todo, to)
	case "google":
		done, err = t.google(ctx, key, todo, to)
	default:
		done, err = t.myMemory(ctx, todo, to)
	}
	if err != nil {
		return "", err
	}
	if len(done) != len(todo) {
		return "", translateErr("failed", "the translation service answered something else")
	}
	out := make([]string, len(parts))
	k := 0
	for i, p := range parts {
		if code[i] {
			out[i] = p
		} else {
			out[i] = done[k]
			k++
		}
	}
	return strings.Join(out, "\n\n"), nil
}

func (t *translator) do(req *http.Request, v any) error {
	res, err := t.http.Do(req)
	if err != nil {
		return translateErr("unreachable", "could not reach the translation service")
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return translateErr("unauthorized", "the translation service did not accept the key")
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 456: // DeepL: quota used up
		return translateErr("quota", "the translation service's limit is used up for now")
	case res.StatusCode >= 300:
		return translateErr("failed", fmt.Sprintf("the translation service answered HTTP %d", res.StatusCode))
	}
	if err := json.Unmarshal(body, v); err != nil {
		return translateErr("failed", "the translation service answered something else")
	}
	return nil
}

func (t *translator) myMemory(ctx context.Context, todo []string, to string) ([]string, error) {
	pair := "en|zh-CN"
	if to == "en" {
		pair = "zh-CN|en"
	}
	out := make([]string, 0, len(todo))
	for _, p := range todo {
		var got []string
		for _, piece := range pieces(p, 450) { // it takes 500 bytes at a time
			q := url.Values{"q": {piece}, "langpair": {pair}}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, translateEndpoints.mymemory+"?"+q.Encode(), nil)
			var r struct {
				ResponseData struct {
					TranslatedText string `json:"translatedText"`
				} `json:"responseData"`
				ResponseStatus any  `json:"responseStatus"`
				QuotaFinished  bool `json:"quotaFinished"`
			}
			if err := t.do(req, &r); err != nil {
				return nil, err
			}
			text := r.ResponseData.TranslatedText
			if r.QuotaFinished || strings.HasPrefix(text, "MYMEMORY WARNING") {
				return nil, translateErr("quota", "the free translation limit for today is used up")
			}
			if s := fmt.Sprint(r.ResponseStatus); s != "200" {
				return nil, translateErr("failed", "the translation service could not translate this")
			}
			got = append(got, text)
		}
		sep := " "
		if to == "zh" {
			sep = ""
		}
		out = append(out, strings.Join(got, sep))
	}
	return out, nil
}

func (t *translator) deepL(ctx context.Context, key string, todo []string, to string) ([]string, error) {
	endpoint := translateEndpoints.deeplPro
	if strings.HasSuffix(key, ":fx") {
		endpoint = translateEndpoints.deeplFree
	}
	target := "ZH-HANS"
	if to == "en" {
		target = "EN-US"
	}
	body, _ := json.Marshal(map[string]any{"text": todo, "target_lang": target})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "DeepL-Auth-Key "+key)
	req.Header.Set("Content-Type", "application/json")
	var r struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := t.do(req, &r); err != nil {
		return nil, err
	}
	out := make([]string, len(r.Translations))
	for i, x := range r.Translations {
		out[i] = x.Text
	}
	return out, nil
}

func (t *translator) google(ctx context.Context, key string, todo []string, to string) ([]string, error) {
	target := "zh-CN"
	if to == "en" {
		target = "en"
	}
	body, _ := json.Marshal(map[string]any{"q": todo, "target": target, "format": "text"})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, translateEndpoints.google, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", key)
	var r struct {
		Data struct {
			Translations []struct {
				TranslatedText string `json:"translatedText"`
			} `json:"translations"`
		} `json:"data"`
	}
	if err := t.do(req, &r); err != nil {
		return nil, err
	}
	out := make([]string, len(r.Data.Translations))
	for i, x := range r.Data.Translations {
		out[i] = x.TranslatedText
	}
	return out, nil
}

func (t *translator) openAI(ctx context.Context, c TranslateConfig, key, text, to string) (string, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := c.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	lang := "Simplified Chinese"
	if to == "en" {
		lang = "English"
	}
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You translate messages between software agents and people. Translate the user's message into " + lang +
				". Keep Markdown, code blocks, inline code, commands, file paths, URLs and names exactly as they are. Answer with the translation only."},
			{"role": "user", "content": text},
		},
		"temperature": 0,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := t.do(req, &r); err != nil {
		return "", err
	}
	if len(r.Choices) == 0 || strings.TrimSpace(r.Choices[0].Message.Content) == "" {
		return "", translateErr("failed", "the translation service answered nothing")
	}
	return strings.TrimSpace(r.Choices[0].Message.Content), nil
}
