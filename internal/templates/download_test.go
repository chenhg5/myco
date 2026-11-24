package templates_test

import (
	"github.com/myco-ai/myco/internal/release"
	"github.com/myco-ai/myco/internal/templates"
	"testing"
)

func TestApplicationDownloadSourceIsIndependent(t *testing.T) {
	for _, test := range []struct {
		values map[string]string
		want   string
	}{
		{map[string]string{}, "https://github.com/myco-ai/apps/releases/download"},
		{map[string]string{templates.EnvDownload: " https://mirror.example/apps/ "}, "https://mirror.example/apps"},
		{map[string]string{release.EnvDownload: "https://legacy.example/download/"}, "https://legacy.example/download"},
		{map[string]string{templates.EnvDownload: "https://apps.example", release.EnvDownload: "https://binary.example"}, "https://apps.example"},
	} {
		get := func(key string) string { return test.values[key] }
		if got := templates.DownloadURL(get); got != test.want {
			t.Fatalf("app download %q, want %q", got, test.want)
		}
		if test.values[templates.EnvDownload] != "" && test.values[release.EnvDownload] == "" && release.DownloadURL(get) != release.DefaultDownload {
			t.Fatal("app override changed program upgrades")
		}
	}
}
