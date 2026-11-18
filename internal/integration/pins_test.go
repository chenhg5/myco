package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// An agent pins a link, a file and a message to its conversation with a
// person (myco pin); the person sees the same pins (the App's console
// token), renames, orders and unpins them. An agent pins in its own
// conversations only.
func TestPinsSharedByBothSides(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	coder := newCLI(t, h, filepath.Join(home, "coder.yaml"))
	coder.must(c, "agent", "register", "--name", "coder")

	mine, err := admin.Pins(c, "", "coder")
	if err != nil || len(mine.Pins) != 0 || mine.A == "" || mine.Max != app.MaxPins {
		t.Fatalf("at first: %v %+v", err, mine)
	}
	person := mine.A // the console's sender, e.g. @console:owner

	var link api.Pin
	if err := json.Unmarshal([]byte(coder.must(c, "pin", "add", "--with", person, "--url", "https://example.com/spec", "--title", "设计稿")), &link); err != nil {
		t.Fatal(err)
	}
	doc := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(doc, []byte("# plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	coder.must(c, "pin", "add", "--with", person, "--file", doc)
	coder.must(c, "pin", "add", "--with", person, "--message", "msg_abc", "--title", "上线计划")
	// --dry-run changes nothing; exactly one thing is pinned at a time.
	if _, out, _ := coder.run(c, "pin", "add", "--with", person, "--url", "https://example.com/x", "--dry-run"); !strings.Contains(out, `"pin_add"`) {
		t.Fatalf("dry run: %s", out)
	}
	if code, _, _ := coder.run(c, "pin", "add", "--with", person, "--url", "https://a.example", "--message", "msg_x"); code == 0 {
		t.Fatal("two things at once should be refused")
	}

	got, err := admin.Pins(c, "", "coder")
	if err != nil || len(got.Pins) != 3 {
		t.Fatalf("the person's view: %v %+v", err, got)
	}
	if got.Pins[0].Title != "设计稿" || got.Pins[0].AddedBy != "coder" || got.Pins[1].Kind != "file" || got.Pins[1].File == nil || got.Pins[1].File.Name != "plan.md" || got.Pins[2].Target != "msg_abc" {
		t.Fatalf("pins: %+v", got.Pins)
	}
	// The person pins one too, renames, orders, unpins.
	p, err := admin.AddPin(c, api.PinCreate{With: "coder", Kind: "link", Target: "https://example.com/preview"})
	if err != nil || p.AddedBy != person {
		t.Fatalf("person pins: %v %+v", err, p)
	}
	if _, err := admin.RenamePin(c, p.ID, api.PinUpdate{With: "coder", Title: "预览"}); err != nil {
		t.Fatal(err)
	}
	ordered, err := admin.OrderPins(c, api.PinOrder{With: "coder", IDs: []string{p.ID}})
	if err != nil || ordered.Pins[0].Title != "预览" || len(ordered.Pins) != 4 {
		t.Fatalf("order: %v %+v", err, ordered)
	}
	if err := admin.RemovePin(c, "", "coder", link.ID); err != nil {
		t.Fatal(err)
	}
	var seen api.PinList
	if err := json.Unmarshal([]byte(coder.must(c, "pin", "list", "--with", person)), &seen); err != nil || len(seen.Pins) != 3 || seen.Pins[0].Title != "预览" {
		t.Fatalf("the agent's view: %v %+v", err, seen)
	}

	// Another agent cannot pin in coder's conversation.
	eveTok := h.register(api.RegisterRequest{Name: "eve"})
	eve := h.client(eveTok)
	_, err = eve.AddPin(c, api.PinCreate{A: "coder", With: person, Kind: "link", Target: "https://evil.example"})
	if e, ok := client.AsError(err); !ok || e.Code != "permission_denied" {
		t.Fatalf("someone else's conversation: %v", err)
	}
	if l, err := eve.Pins(c, "", person); err != nil || len(l.Pins) != 0 {
		t.Fatalf("eve's own conversation: %v %+v", err, l)
	}
}
