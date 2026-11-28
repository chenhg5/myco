package uilocale

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCatalogsAndFallback(t *testing.T) {
	for _, language := range []string{"zh-Hant", "ja", "ko", "es", "pt-BR"} {
		t.Run(language, func(t *testing.T) {
			raw, err := files.ReadFile("catalogs/" + language + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var catalog map[string]string
			if err = json.Unmarshal(raw, &catalog); err != nil {
				t.Fatal(err)
			}
			if len(catalog) < 1000 {
				t.Fatalf("incomplete catalog: %d", len(catalog))
			}
			if got := Text(language, "设置", "Settings"); got == "" || got == "Settings" {
				t.Errorf("missing core translation: %s", got)
			}
			format := Text(language, "有新版本 %s，点一下更新", "myco %s is out: update")
			if got := fmt.Sprintf(format, "v1.2.3"); !strings.Contains(got, "v1.2.3") || strings.Contains(got, "%!") {
				t.Fatalf("version placeholder: %s", got)
			}
			if got := Text(language, "未收录的中文", "Uncataloged example"); got != "Uncataloged example" {
				t.Fatalf("missing-entry fallback: %s", got)
			}
		})
	}
	if got := Text("fr", "中文", "English"); got != "English" {
		t.Fatal(got)
	}
	if got := Text("zh", "中文", "English"); got != "中文" {
		t.Fatal(got)
	}
}
