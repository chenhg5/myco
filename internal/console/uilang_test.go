package console

import (
	"strings"
	"testing"
	"time"
)

// The pages say their language; the desktop app hears it, and it is kept
// for next time.
func TestUILangIsKeptAndHeard(t *testing.T) {
	dir := t.TempDir()
	heard := make(chan string, 1)
	b := &Backend{Store: NewStore(dir + "/hubs.json"), Secrets: &memKeys{m: map[string]string{}}, OnUILang: func(l string) { heard <- l }}
	srv := httptestServer(t, b)
	if code, body := srv("GET", "/app/ui-lang", ""); code != 200 || !strings.Contains(body, `"lang":""`) {
		t.Fatalf("before: %d %s", code, body)
	}
	if code, _ := srv("PUT", "/app/ui-lang", `{"lang":"fr"}`); code != 400 {
		t.Fatalf("an unknown language: %d", code)
	}
	if code, body := srv("PUT", "/app/ui-lang", `{"lang":"en"}`); code != 200 || !strings.Contains(body, `"lang":"en"`) {
		t.Fatalf("put: %d %s", code, body)
	}
	select {
	case l := <-heard:
		if l != "en" {
			t.Fatalf("heard %q", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the app was not told")
	}
	// Next start: the same language, before the pages say anything.
	b2 := &Backend{Store: NewStore(dir + "/hubs.json")}
	if got := b2.UILang(); got != "en" {
		t.Fatalf("kept: %q", got)
	}
}

func TestUILangAllSupportedLanguagesSurviveRestart(t *testing.T) {
	for _, language := range []string{"zh", "zh-Hant", "en", "ja", "ko", "es", "pt-BR"} {
		t.Run(language, func(t *testing.T) {
			dir := t.TempDir()
			heard := make(chan string, 1)
			b := &Backend{Store: NewStore(dir + "/hubs.json"), OnUILang: func(l string) { heard <- l }}
			request := httptestServer(t, b)
			if code, body := request("PUT", "/app/ui-lang", `{"lang":"`+language+`"}`); code != 200 {
				t.Fatalf("put: %d %s", code, body)
			}
			select {
			case got := <-heard:
				if got != language {
					t.Fatalf("callback: %s", got)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("no callback")
			}
			restarted := &Backend{Store: NewStore(dir + "/hubs.json")}
			if got := restarted.UILang(); got != language {
				t.Fatalf("restart: %s", got)
			}
		})
	}
}
