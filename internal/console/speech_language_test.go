package console

import (
	"strings"
	"testing"
)

func TestNonChineseDictationDoesNotEnableChineseAliases(t *testing.T) {
	for _, language := range []string{"ja-JP", "ko-KR", "es-ES", "pt-BR"} {
		fixer := newSpeechFixer(language, commonSpeechWords, nil)
		original := "克劳德 cloud 句号"
		if got := fixer.Fix(original); got != original {
			t.Errorf("%s enabled Chinese-only fixes: %q", language, got)
		}
		if !strings.HasPrefix(polishPrompt(language, []string{"Claude"}), "You correct speech recognition.") {
			t.Errorf("%s did not get language-neutral correction instructions", language)
		}
	}
	if got := newSpeechFixer("zh-TW", commonSpeechWords, nil).Fix("讓克劳德看一下"); !strings.Contains(got, "Claude") {
		t.Fatal("Chinese aliases no longer supported")
	}
}
