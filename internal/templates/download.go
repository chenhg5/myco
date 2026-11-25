package templates

import (
	"strings"

	"github.com/myco-ai/myco/internal/release"
)

const DefaultDownload = "https://github.com/myco-ai/apps/releases/download"
const EnvDownload = "MYCO_APPS_DOWNLOAD"

// DownloadURL separates application updates from program upgrades. The legacy
// shared override still works; an explicit app override takes precedence.
func DownloadURL(getenv func(string) string) string {
	if value := strings.TrimSpace(getenv(EnvDownload)); value != "" {
		return strings.TrimRight(value, "/")
	}
	if value := strings.TrimSpace(getenv(release.EnvDownload)); value != "" {
		return strings.TrimRight(value, "/")
	}
	return DefaultDownload
}
