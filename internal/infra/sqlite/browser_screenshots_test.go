package sqlite

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestBrowserScreenshotRetention(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "browser-screenshots.sqlite")
	repo, err := OpenBrowserScreenshots(path)
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	a := domain.BrowserScreenshot{
		ID: "art", AgentID: "agent-id", OwnerID: "owner-id",
		ExpiresAt: exp, MediaType: "image/png", Data: []byte("image data"),
	}
	err = repo.Save(ctx, a)
	closeErr := repo.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("save and close: %v, %v", err, closeErr)
	}
	repo, err = OpenBrowserScreenshots(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	before := exp.Add(-time.Millisecond)
	for _, actor := range [][2]string{{a.AgentID, ""}, {"", a.OwnerID}} {
		got, err := repo.Get(ctx, a.ID, actor[0], actor[1], before)
		if err != nil || !bytes.Equal(got.Data, a.Data) || !got.ExpiresAt.Equal(exp) {
			t.Fatalf("retained screenshot after restart: %+v, %v", got, err)
		}
	}
	if _, err := repo.Get(ctx, a.ID, a.AgentID, "", exp); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("expired image accessible: %v", err)
	}
	if err := repo.Prune(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, a.ID, a.AgentID, "", before); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("expired image still stored after pruning: %v", err)
	}
}
