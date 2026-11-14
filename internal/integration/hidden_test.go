package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
)

// A person takes messages out of their own view: every device of theirs
// sees the same list, someone else's list is their own, and showing one
// again brings it back. Nothing is deleted.
func TestHiddenMessagesPerPerson(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()

	empty, err := admin.Hidden(c)
	if err != nil || len(empty.IDs) != 0 {
		t.Fatalf("at first: %v %+v", err, empty)
	}
	got, err := admin.UpdateHidden(c, api.HiddenUpdate{Hide: []string{"msg_a", "msg_b", "msg_a"}})
	if err != nil || strings.Join(got.IDs, ",") != "msg_a,msg_b" {
		t.Fatalf("hide: %v %+v", err, got)
	}
	// Another person has a list of their own.
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "laohu"})
	if err != nil {
		t.Fatal(err)
	}
	paired, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "laohu-mac")
	if err != nil {
		t.Fatal(err)
	}
	laohu := h.client(paired.Token)
	if l, err := laohu.Hidden(c); err != nil || len(l.IDs) != 0 {
		t.Fatalf("someone else's list: %v %+v", err, l)
	}
	if _, err := laohu.UpdateHidden(c, api.HiddenUpdate{Hide: []string{"msg_c"}}); err != nil {
		t.Fatal(err)
	}
	// Shown again: gone from the list; the other person's list is untouched.
	got, err = admin.UpdateHidden(c, api.HiddenUpdate{Show: []string{"msg_a"}})
	if err != nil || strings.Join(got.IDs, ",") != "msg_b" {
		t.Fatalf("show: %v %+v", err, got)
	}
	if l, _ := laohu.Hidden(c); strings.Join(l.IDs, ",") != "msg_c" {
		t.Fatalf("laohu's list: %+v", l)
	}
	// Too many at once is refused.
	many := make([]string, 501)
	for i := range many {
		many[i] = "msg_x"
	}
	if _, err := admin.UpdateHidden(c, api.HiddenUpdate{Hide: many}); err == nil {
		t.Fatal("501 at once should be refused")
	}
}
