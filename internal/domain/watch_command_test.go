package domain

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestCommandWatchRedaction(t *testing.T) {
	for _, secret := range []string{"myco_token_example", "sk-example-secret", "ghp_abc123", "github_pat_example_123", "xoxb-123-abcdef", "Bearer test-value", "api_key=abc123", "Authorization: Bearer short", "Authorization: Basic dXNlcjpwdw==", `"password":"two words"`, "https://demo:short@example.com", "eyJhbGciOiJYIn0.e30.abcdef", `"password":"abc123"`, strings.Repeat("Z", 80)} {
		got := SanitizeWatchResult("old line\nready " + secret)
		if strings.Contains(got, secret) || !strings.Contains(got, "[redacted]") || strings.Contains(got, "\n") {
			t.Errorf("unsafe summary: %q", got)
		}
	}
	got := SanitizeWatchResult("first\r\n\x1b[31mready\x1b[0m\n")
	if got != "ready" {
		t.Fatalf("last printable line: %q", got)
	}
	got = SanitizeWatchResult(strings.Repeat("中文", 1000))
	if len(got) > 1024 || !utf8.ValidString(got) {
		t.Fatal("unbounded/invalid summary")
	}
	if CommandCheckTimeout(30) != 15*time.Second || CommandCheckTimeout(600) != 60*time.Second {
		t.Fatal("bad timeout budget")
	}
}
