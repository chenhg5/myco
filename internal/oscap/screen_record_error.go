package oscap

import (
	"fmt"
	"strings"
)

// stderr is tool diagnostics, not the captured pixels. Bound it and remove the
// local movie path. Only a quiet user cancellation is treated as cancelled.
func screenCaptureFailure(err error, diagnostic, path string) error {
	detail := strings.TrimSpace(strings.ReplaceAll(diagnostic, path, "<recording>"))
	if len(detail) > 2048 {
		detail = detail[:2048] + "…"
	}
	if detail == "" {
		return errf(CodeCancelled, "screen recording ended without a movie; select a region and start recording again")
	}
	return errf(CodeFailed, "macOS screen recording failed (%s): %s", fmt.Sprint(err), detail)
}
