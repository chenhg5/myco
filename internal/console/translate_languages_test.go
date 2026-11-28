package console

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestTranslateSupportedLanguageTargets(t *testing.T) {
	for _, language := range []string{"zh", "zh-Hant", "en", "ja", "ko", "es", "pt-BR"} {
		if !api.UILanguage(language).Valid() {
			t.Fatal(language)
		}
		target, ok := translationTargets[language]
		if !ok || target.deepL == "" || target.google == "" || target.name == "" {
			t.Fatalf("missing target %s", language)
		}
		t.Run(language, func(t *testing.T) {
			var request map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"choices":[{"message":{"content":"translated"}}]}`))
			}))
			defer server.Close()
			translator := &translator{http: server.Client()}
			got, err := translator.openAI(context.Background(), TranslateConfig{BaseURL: server.URL, Model: "test-model"}, "test-key", "original", language)
			if err != nil || got != "translated" {
				t.Fatalf("%q %v", got, err)
			}
			messages := request["messages"].([]any)
			if !strings.Contains(messages[0].(map[string]any)["content"].(string), target.name) {
				t.Fatal("wrong target prompt")
			}
			if messages[1].(map[string]any)["content"] != "original" {
				t.Fatal("original changed")
			}
		})
	}
	_, handler, _ := translateBackend(t)
	if code, _ := call(t, handler, "POST", "/app/translate", `{"text":"original","to":"system"}`); code != 400 {
		t.Fatal(code)
	}
	if code, _ := call(t, handler, "POST", "/app/translate", `{"text":"original","to":"ja"}`); code != 409 {
		t.Fatalf("valid language should require configured key, got %d", code)
	}
}

type languageTransport func(*http.Request) (*http.Response, error)

func (f languageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTranslateProviderLanguageCodes(t *testing.T) {
	for language, target := range translationTargets {
		for _, provider := range []string{"deepl", "google"} {
			t.Run(language+"/"+provider, func(t *testing.T) {
				httpClient := &http.Client{Transport: languageTransport(func(r *http.Request) (*http.Response, error) {
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					field, want, body := "target_lang", target.deepL, `{"translations":[{"text":"translated"}]}`
					if provider == "google" {
						field, want, body = "target", target.google, `{"data":{"translations":[{"translatedText":"translated"}]}}`
					}
					if payload[field] != want {
						t.Fatalf("target got %v, want %s", payload[field], want)
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
				})}
				tr := &translator{http: httpClient}
				got, err := tr.translate(context.Background(), TranslateConfig{Provider: provider}, "test-key", "original", language)
				if err != nil || got != "translated" {
					t.Fatalf("%q %v", got, err)
				}
			})
		}
	}
}
