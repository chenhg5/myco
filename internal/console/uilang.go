package console

import (
	"net/http"

	"github.com/chenhg5/myco/internal/client"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The language the App's pages show (Settings › Appearance), for what the
// desktop app draws itself (the menu-bar or tray menu). The pages say it
// when they load and when it changes; it is kept on this computer, so the
// menu starts in it next time.
//
//	GET /app/ui-lang         {lang}   "zh", "en" or "" (not said yet)
//	PUT /app/ui-lang {lang}  {lang}

type uiLang struct {
	mu   sync.Mutex
	once sync.Once
	path string
	lang string
}

// UILang is the language the pages last said ("zh", "en", or "" when they
// have not said yet: the system's, then).
func (b *Backend) UILang() string {
	u := b.uiLangState()
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lang
}

func (b *Backend) uiLangState() *uiLang {
	u := &b.uiLang
	u.once.Do(func() {
		if b.Store != nil {
			u.path = filepath.Join(filepath.Dir(b.Store.path), "console-ui-lang")
			if data, err := os.ReadFile(u.path); err == nil {
				u.lang = cleanUILang(string(data))
			}
		}
	})
	return u
}

func cleanUILang(s string) string {
	switch s = strings.TrimSpace(s); s {
	case "zh", "en":
		return s
	}
	return ""
}

func (b *Backend) uiLangRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/ui-lang", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"lang": b.UILang()})
	})
	m.HandleFunc("PUT /app/ui-lang", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Lang string `json:"lang"`
		}
		if !decode(w, r, &in) {
			return
		}
		l := cleanUILang(in.Lang)
		if l == "" {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "lang is zh or en"})
			return
		}
		u := b.uiLangState()
		u.mu.Lock()
		changed := u.lang != l
		u.lang = l
		if changed && u.path != "" {
			_ = writeFileAtomic(u.path, []byte(l))
		}
		u.mu.Unlock()
		if changed && b.OnUILang != nil {
			go b.OnUILang(l)
		}
		writeJSON(w, http.StatusOK, map[string]string{"lang": l})
	})
}
