package codex

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/provider"
)

func TestPinnedProviderOverridesLocalArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake CLI")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "codex")
	capture := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_ARGS\"\ntest \"$MYCO_CODEX_API_KEY\" = 'secret-env-only' || exit 9\nprintf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"ok\"}}'\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	h, _ := New(harness.Options{Binary: binary, Args: []string{"-c", `model_provider="wrong"`}})
	args, env := provider.CustomCodex("http://127.0.0.1:3425/v1", "secret-env-only", "m", "responses")
	_, err := h.Run(context.Background(), harness.Request{Workdir: dir, Prompt: "hello", Env: []string{"CAPTURE_ARGS=" + capture, "MYCO_CODEX_API_KEY=wrong-env"}, Provider: &harness.ProviderSettings{ConfigArgs: args, Env: env}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(capture)
	text := string(raw)
	if strings.Contains(text, "secret-env-only") || strings.LastIndex(text, `model_provider="myco_custom"`) < strings.LastIndex(text, `model_provider="wrong"`) {
		t.Fatal("custom provider did not override local arguments safely")
	}
}
