package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestPinsBothSidesAddRenameOrderRemove(t *testing.T) {
	ctx := context.Background()
	db, _ := sqlite.Open(":memory:")
	defer db.Close()
	ps := app.NewPinService(sqlite.NewSettingRepo(db), security.IDs{}, &fakeClock{t: time.Unix(1_000_000, 0)})

	doc, err := ps.Add(ctx, "@console:ann", "coder", app.AddPin{Kind: app.PinKindLink, Target: "https://example.com/spec/", By: "@console:ann"})
	if err != nil || doc.Title != "example.com/spec" || doc.ID == "" {
		t.Fatalf("link: %+v %v", doc, err)
	}
	// The agent's side names the same conversation.
	msg, err := ps.Add(ctx, "coder", "@console:ann", app.AddPin{Kind: app.PinKindMessage, Target: "msg_1", Title: "  the   plan ", By: "coder"})
	if err != nil || msg.Title != "the plan" {
		t.Fatalf("message: %+v %v", msg, err)
	}
	file, err := ps.Add(ctx, "coder", "@console:ann", app.AddPin{Kind: app.PinKindFile, File: &app.PinFile{ID: "f_1", Name: "report.pdf"}, By: "coder"})
	if err != nil || file.Title != "report.pdf" {
		t.Fatalf("file: %+v %v", file, err)
	}
	list, _ := ps.Pins(ctx, "@console:ann", "coder")
	if len(list) != 3 || list[0].ID != doc.ID || list[2].ID != file.ID {
		t.Fatalf("list: %+v", list)
	}
	// Pinned again: not twice; a new title is taken.
	again, err := ps.Add(ctx, "coder", "@console:ann", app.AddPin{Kind: app.PinKindLink, Target: "https://example.com/spec/", Title: "Spec", By: "coder"})
	if err != nil || again.ID != doc.ID || again.Title != "Spec" {
		t.Fatalf("again: %+v %v", again, err)
	}
	if list, _ := ps.Pins(ctx, "coder", "@console:ann"); len(list) != 3 {
		t.Fatalf("pinned twice: %d", len(list))
	}
	if _, err := ps.Rename(ctx, "coder", "@console:ann", msg.ID, "Plan v2"); err != nil {
		t.Fatal(err)
	}
	list, err = ps.Order(ctx, "@console:ann", "coder", []string{file.ID, msg.ID})
	if err != nil || list[0].ID != file.ID || list[1].Title != "Plan v2" || list[2].ID != doc.ID {
		t.Fatalf("order: %+v %v", list, err)
	}
	if err := ps.Remove(ctx, "coder", "@console:ann", msg.ID); err != nil {
		t.Fatal(err)
	}
	if err := ps.Remove(ctx, "coder", "@console:ann", msg.ID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("removed twice: %v", err)
	}
	// Another conversation has its own.
	if list, _ := ps.Pins(ctx, "coder", "reviewer"); len(list) != 0 {
		t.Fatalf("other conversation: %+v", list)
	}
}

func TestPinsRefuseBadInputAndKeepAtMost(t *testing.T) {
	ctx := context.Background()
	db, _ := sqlite.Open(":memory:")
	defer db.Close()
	ps := app.NewPinService(sqlite.NewSettingRepo(db), security.IDs{}, &fakeClock{t: time.Unix(1_000_000, 0)})
	bad := []app.AddPin{
		{Kind: app.PinKindLink, Target: "javascript:alert(1)"},
		{Kind: app.PinKindLink, Target: "file:///etc/passwd"},
		{Kind: app.PinKindMessage},
		{Kind: app.PinKindFile},
		{Kind: "other", Target: "x"},
	}
	for _, in := range bad {
		if _, err := ps.Add(ctx, "a", "b", in); !domain.IsCode(err, domain.CodeInvalidArgument) {
			t.Fatalf("%+v: %v", in, err)
		}
	}
	if _, err := ps.Pins(ctx, "a", "a"); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatalf("with itself: %v", err)
	}
	for i := 0; i < app.MaxPins; i++ {
		if _, err := ps.Add(ctx, "a", "b", app.AddPin{Kind: app.PinKindMessage, Target: fmt.Sprintf("msg_%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ps.Add(ctx, "a", "b", app.AddPin{Kind: app.PinKindMessage, Target: "msg_more"}); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("over the most: %v", err)
	}
}
