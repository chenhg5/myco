package app_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/blob"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

func TestFileLimitsExpiryAndSweep(t *testing.T) {
	ctx := context.Background()
	db, _ := sqlite.Open(":memory:")
	defer db.Close()
	clock := &fakeClock{t: time.Unix(1_000_000, 0)}
	dir := t.TempDir()
	fs := app.NewFileService(sqlite.NewFileRepo(db), blob.FS{Dir: dir}, security.IDs{}, clock,
		app.FilePolicy{MaxSize: 10, TTL: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	alice := &domain.Agent{Name: "alice"}

	if _, err := fs.Upload(ctx, alice, "big.bin", "", strings.NewReader("01234567890")); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatalf("over max size: %v", err)
	}
	f, err := fs.Upload(ctx, alice, "../../etc/passwd", "text/plain", strings.NewReader("hi"))
	if err != nil || f.Name != "passwd" || f.Size != 2 {
		t.Fatalf("upload: %+v %v", f, err)
	}
	if _, _, err := fs.Open(ctx, &domain.Agent{Name: "eve"}, f.ID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("non-owner: %v", err)
	}
	clock.t = clock.t.Add(2 * time.Hour)
	if _, _, err := fs.Open(ctx, alice, f.ID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("expired: %v", err)
	}
	if n, err := fs.Sweep(ctx); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	if _, err := fs.Meta(ctx, alice, f.ID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("after sweep: %v", err)
	}
}
