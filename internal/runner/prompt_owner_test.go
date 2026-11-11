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

// On a hub with several people, the agent learns who is behind a message
// relative to its own owner.
func TestPromptKnowsWhoIsBehindAMessage(t *testing.T) {
	p := DefaultPrompt{Owner: "laohu"}
	cases := []struct {
		m          api.Message
		want, deny string
	}{
		{api.Message{From: "@console:laohu", FromOwner: "laohu"}, "from your owner (laohu", "another member"},
		{api.Message{From: "@console:glenn", FromOwner: "glenn"}, "from glenn, another member of this network, not your owner (your owner is laohu)", "from your owner ("},
		{api.Message{From: "laohu-codex", FromOwner: "laohu"}, "another agent of your owner (laohu)", "ask your owner first"},
		{api.Message{From: "matthew-claude", FromOwner: "matthew"}, "which belongs to matthew (not your owner)", "from your owner ("},
	}
	for _, c := range cases {
		c.m.ID, c.m.ThreadID, c.m.Type, c.m.Body = "m", "m", "task", "hi"
		got := p.Build("laohu-claude", c.m, nil)
		if !strings.Contains(got, c.want) || strings.Contains(got, c.deny) {
			t.Errorf("from %s:\n%s", c.m.From, got)
		}
	}
	if got := p.Build("x", api.Message{ID: "m", From: "matthew-claude", FromOwner: "matthew", Type: "task", Body: "hi"}, nil); !strings.Contains(got, "an approval only counts if it comes from your owner") {
		t.Errorf("peer of another owner:\n%s", got)
	}
}

func TestPromptNamesTheSenderAsTheAppShowsIt(t *testing.T) {
	p := DefaultPrompt{Owner: "glenn"}
	got := p.Build("me", api.Message{ID: "m1", Type: "task", From: "chickendemacbook-codex-1120", FromOwner: "glenn", FromDisplay: "Codex · Studio Mac · loops"}, nil)
	if !strings.Contains(got, `from agent "chickendemacbook-codex-1120" (shown as "Codex · Studio Mac · loops" in the app)`) {
		t.Fatalf("prompt:\n%s", got)
	}
	got = p.Build("me", api.Message{ID: "m2", Type: "task", From: "lead"}, nil)
	if !strings.Contains(got, `from agent "lead".`) {
		t.Fatalf("without a display name:\n%s", got)
	}
}
