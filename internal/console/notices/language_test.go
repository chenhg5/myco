package notices

import (
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestLocalizedNoticePreservesOriginalMessage(t *testing.T) {
	for _, language := range []string{"zh", "zh-Hant", "en", "ja", "ko", "es", "pt-BR"} {
		original := "Demo <record> {0} — 原文 /tmp/example"
		got := WordsForLanguage(&api.Notice{Kind: "asking", Subject: original}, language)
		if !strings.HasSuffix(got, original) || got == original {
			t.Errorf("%s: prefix or original changed: %q", language, got)
		}
	}
}
