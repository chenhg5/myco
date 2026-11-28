package api

import "testing"

func TestUILanguageAllowlist(t *testing.T) {
	for _, language := range []UILanguage{"zh", "zh-Hant", "en", "ja", "ko", "es", "pt-BR"} {
		if !language.Valid() {
			t.Errorf("supported language rejected: %s", language)
		}
	}
	for _, language := range []UILanguage{"", "system", "fr", "ja-JP", "../en"} {
		if language.Valid() {
			t.Errorf("unsupported language accepted: %s", language)
		}
	}
}
