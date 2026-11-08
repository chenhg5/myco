package blob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/testutil"
)

func readAll(t *testing.T, s FS, id string) string {
	t.Helper()
	rc, err := s.Open(context.Background(), id)
	if err != nil {
		t.Fatalf("open %s: %v", id, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPutOpenDeleteRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := FS{Dir: t.TempDir()}
	body := "hello, blob\n"
	n, sum, err := s.Put(ctx, "file_abc123", strings.NewReader(body), 1024)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(body))
	if n != int64(len(body)) || sum != hex.EncodeToString(want[:]) {
		t.Fatalf("Put = (%d, %s), want (%d, %x)", n, sum, len(body), want)
	}
	if got := readAll(t, s, "file_abc123"); got != body {
		t.Fatalf("content = %q", got)
	}
	// Sharded by the last two characters of the id.
	if _, err := os.Stat(filepath.Join(s.Dir, "23", "file_abc123")); err != nil {
		t.Fatalf("expected sharded layout: %v", err)
	}
	// Overwrite replaces the content.
	if _, _, err := s.Put(ctx, "file_abc123", strings.NewReader("v2"), 1024); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, s, "file_abc123"); got != "v2" {
		t.Fatalf("after overwrite = %q", got)
	}
	if err := s.Delete(ctx, "file_abc123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, "file_abc123"); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("open after delete: want not_found, got %v", err)
	}
	if err := s.Delete(ctx, "file_abc123"); err != nil {
		t.Fatalf("delete of a missing file must be a no-op: %v", err)
	}
	assertNoTempFiles(t, s.Dir)
}

func TestPutSizeLimit(t *testing.T) {
	ctx := context.Background()
	s := FS{Dir: t.TempDir()}
	n, _, err := s.Put(ctx, "file_exact", strings.NewReader("12345"), 5)
	if err != nil || n != 5 {
		t.Fatalf("exactly max bytes: n=%d err=%v", n, err)
	}
	_, _, err = s.Put(ctx, "file_big", strings.NewReader("123456"), 5)
	if !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatalf("over max: want invalid_argument, got %v", err)
	}
	if _, err := s.Open(ctx, "file_big"); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("rejected upload must not be stored: %v", err)
	}
	n, _, err = s.Put(ctx, "file_empty", strings.NewReader(""), 5)
	if err != nil || n != 0 {
		t.Fatalf("empty upload: n=%d err=%v", n, err)
	}
	assertNoTempFiles(t, s.Dir)
}

func TestPutReaderErrorLeavesNothing(t *testing.T) {
	ctx := context.Background()
	s := FS{Dir: t.TempDir()}
	r := io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(errors.New("conn reset")))
	if _, _, err := s.Put(ctx, "file_x1", r, 1024); err == nil || !strings.Contains(err.Error(), "conn reset") {
		t.Fatalf("want reader error, got %v", err)
	}
	if _, err := s.Open(ctx, "file_x1"); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("failed upload must not be stored: %v", err)
	}
	assertNoTempFiles(t, s.Dir)
}

func TestOpenMissing(t *testing.T) {
	s := FS{Dir: t.TempDir()}
	if _, err := s.Open(context.Background(), "file_nope"); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("want not_found, got %v", err)
	}
}

func TestInvalidIDsCannotEscapeDir(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	base := filepath.Join(root, "blobs")
	s := FS{Dir: base}
	bad := []string{
		"", "x", "../x", "file_../../x", "../../etc/passwd", "file_ab/../../x",
		"/etc/passwd", "file_AB", "File_ab", "file-ab", "file_ab\x00", "file_a.b",
		"_ab", "file_", "a_b c", `file_ab\..\x`, "file_ab/",
	}
	for _, id := range bad {
		if _, _, err := s.Put(ctx, id, strings.NewReader("pwn"), 1024); !domain.IsCode(err, domain.CodeInvalidArgument) {
			t.Errorf("Put(%q): want invalid_argument, got %v", id, err)
		}
		if _, err := s.Open(ctx, id); !domain.IsCode(err, domain.CodeInvalidArgument) {
			t.Errorf("Open(%q): want invalid_argument, got %v", id, err)
		}
		if err := s.Delete(ctx, id); !domain.IsCode(err, domain.CodeInvalidArgument) {
			t.Errorf("Delete(%q): want invalid_argument, got %v", id, err)
		}
	}
	// Nothing was written anywhere under root.
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("invalid ids created files: %v", entries)
	}
	// Valid ids always resolve inside Dir.
	for _, id := range []string{"a_b", "msg_k3j2", "file_00"} {
		p, err := s.path(id)
		if err != nil {
			t.Fatalf("path(%q): %v", id, err)
		}
		if rel, err := filepath.Rel(base, p); err != nil || strings.HasPrefix(rel, "..") {
			t.Fatalf("path(%q) = %s escapes %s", id, p, base)
		}
	}
}

func TestPermissions(t *testing.T) {
	testutil.NeedsUnixPermissions(t)
	s := FS{Dir: filepath.Join(t.TempDir(), "blobs")}
	if _, _, err := s.Put(context.Background(), "file_perm1", bytes.NewReader([]byte("secret")), 100); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(s.Dir, "m1", "file_perm1"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("blob mode = %o, want 600", perm)
	}
	for _, d := range []string{s.Dir, filepath.Join(s.Dir, "m1")} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o700 {
			t.Errorf("dir %s mode = %o, want 700", d, perm)
		}
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), ".upload-") {
			t.Errorf("temp file left behind: %s", p)
		}
		return nil
	})
}
