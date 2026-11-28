package console

import (
	"context"
	"encoding/json"
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
