package domain

import (
	"strings"
	"testing"
)

func TestTemplateLocales(t *testing.T) {
	locales, err := ParseTemplateLocales([]byte(`{"en":{"name":"Notes","description":"Shared notes"},"zh-CN":{"name":"笔记"}}`))
	if err != nil || locales["en"].Name != "Notes" || locales["zh-CN"].Name != "笔记" {
		t.Fatal(locales, err)
	}
	for _, bad := range []string{`[]`, `{"../../en":{"name":"x"}}`, `{"en":{"name":"two\nlines"}}`, `{"en":{"description":"` + strings.Repeat("x", 301) + `"}}`} {
		if _, err := ParseTemplateLocales([]byte(bad)); err == nil {
			t.Fatal("accepted invalid locales", bad)
		}
	}
	if _, err := ParseTemplateLocales([]byte(strings.Repeat(" ", 33<<10))); err == nil {
		t.Fatal("accepted oversized sidecar")
	}
	if v, err := ParseTemplateLocales(nil); err != nil || v != nil {
		t.Fatal("legacy template changed", v, err)
	}
}
