package runner

import (
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
)

func TestPromptFramesConsoleMessagesAsTheOwners(t *testing.T) {
	owner := DefaultPrompt{}.Build("worker", api.Message{ID: "m1", ThreadID: "m1", From: "@console:glenn-mac", Type: "task", Body: "hi"}, nil)
	if !strings.Contains(owner, "a task from your owner") || strings.Contains(owner, "peer request") {
		t.Fatalf("console message:\n%s", owner)
	}
	peer := DefaultPrompt{}.Build("worker", api.Message{ID: "m2", ThreadID: "m2", From: "lead", Type: "task", Body: "hi"}, nil)
	if !strings.Contains(peer, "peer request") || strings.Contains(peer, "a task from your owner") {
		t.Fatalf("agent message:\n%s", peer)
	}
}
