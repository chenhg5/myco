package console

import (
	"testing"

	"github.com/chenhg5/myco/internal/api"
)

// A shell command that exited non-zero is a failure in the list; the hub
// cannot see the exit code (it is in the reply body, maybe encrypted).
func TestRefineRecentShellExitCode(t *testing.T) {
	pair := func(body, unreadable string) api.RecentItem {
		return api.RecentItem{Kind: "pair", State: "done", Answer: &api.Message{Type: "reply", Body: body, Unreadable: unreadable}}
	}
	for _, c := range []struct {
		it   api.RecentItem
		want string
	}{
		{pair(`{"exit_code":0,"stdout":"ok"}`, ""), "done"},
		{pair(`{"exit_code":1,"stderr":"bad"}`, ""), "failed"},
		{pair(`{"exit_code":2}`, "not sealed for this device"), "done"}, // unreadable: leave it
		{pair(`not json`, ""), "done"},
		{api.RecentItem{Kind: "thread", State: "done", Answer: &api.Message{Type: "reply", Body: `{"exit_code":1}`}}, "done"},
	} {
		it := c.it
		refineRecent(&it)
		if it.State != c.want {
			t.Errorf("%+v: state %q, want %q", c.it.Answer, it.State, c.want)
		}
	}
}

// With end-to-end encryption the app searches the decrypted rows itself,
// including the names of both sides.
func TestFilterRecent(t *testing.T) {
	items := []api.RecentItem{
		{Kind: "pair", Other: "lead", Node: "box", Last: api.Message{Body: `{"exit_code":0}`}},
		{Kind: "thread", Participants: []string{"@console:glenn", "docs"}, First: api.Message{Body: "Write the release notes"}},
	}
	if got := filterRecent(append([]api.RecentItem{}, items...), "BOX"); len(got) != 1 || got[0].Node != "box" {
		t.Fatalf("by node name: %+v", got)
	}
	if got := filterRecent(append([]api.RecentItem{}, items...), "release"); len(got) != 1 || got[0].Kind != "thread" {
		t.Fatalf("by body: %+v", got)
	}
}
