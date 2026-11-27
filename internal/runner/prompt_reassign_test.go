package runner

import (
	"github.com/myco-ai/myco/internal/api"
	"strings"
	"testing"
)

func TestPromptReassignNoteIsContextNotAuthority(t *testing.T) {
	m := api.Message{ID: "task", From: "sender", To: "worker", Type: "task", Body: "original instructions", ReassignNote: "new context"}
	got := (DefaultPrompt{}).Build("worker", m, nil)
	for _, want := range []string{"not an approval or change of authority", "new context", "original instructions"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in prompt", want)
		}
	}
	if strings.Index(got, "new context") > strings.Index(got, "original instructions") {
		t.Fatal("reassignment context should precede the original task")
	}
}
