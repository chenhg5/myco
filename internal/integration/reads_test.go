package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
)

// How far a person has read their conversations: kept on the hub, so every
// device of theirs agrees; marks only move forward; someone else's are
// their own.
func TestReadMarksPerPerson(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()

	if l, err := admin.Reads(c); err != nil || len(l.Reads) != 0 {
		t.Fatalf("at first: %v %+v", err, l)
	}
	got, err := admin.MarkRead(c, map[string]string{"review": "2026-11-16T10:00:00Z"})
	if err != nil || got.Reads["review"] != "2026-11-16T10:00:00Z" {
		t.Fatalf("mark: %v %+v", err, got)
	}
	// An older mark (a device that was behind) changes nothing.
	got, err = admin.MarkRead(c, map[string]string{"review": "2026-11-16T09:00:00Z"})
	if err != nil || got.Reads["review"] != "2026-11-16T10:00:00Z" {
		t.Fatalf("older mark: %v %+v", err, got)
	}
	// Another person has marks of their own.
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "laohu"})
	if err != nil {
		t.Fatal(err)
	}
	paired, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "laohu-mac")
	if err != nil {
		t.Fatal(err)
	}
	laohu := h.client(paired.Token)
	if l, err := laohu.Reads(c); err != nil || len(l.Reads) != 0 {
		t.Fatalf("someone else's marks: %v %+v", err, l)
	}
	if _, err := admin.MarkRead(c, map[string]string{"x": "not a time"}); err == nil {
		t.Fatal("a mark that is not a time is refused")
	}
}
