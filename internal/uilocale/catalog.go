// Package uilocale shares offline display catalogs with the web interface.
package uilocale

import (
	"embed"
	"encoding/json"
	"sync"

	"github.com/myco-ai/myco/internal/api"
)

//go:embed catalogs/*.json
var files embed.FS
var catalogs sync.Map

// Text returns authored Chinese/English or a catalog entry. Values are not translated.
func Text(language, chinese, english string) string {
	if language == "zh" {
		return chinese
	}
	if language == "en" || !api.UILanguage(language).Valid() {
		return english
	}
	cached, ok := catalogs.Load(language)
	if !ok {
		catalog := map[string]string{}
		if raw, err := files.ReadFile("catalogs/" + language + ".json"); err == nil {
			_ = json.Unmarshal(raw, &catalog)
		}
		cached, _ = catalogs.LoadOrStore(language, catalog)
	}
	key := english
	if language == "zh-Hant" {
		key = chinese
	}
	if translated := cached.(map[string]string)[key]; translated != "" {
		return translated
	}
	return english
}
