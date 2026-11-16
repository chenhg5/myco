package app

import (
	"context"
	"testing"
)

type memSettings map[string]string

func (m memSettings) Get(_ context.Context, k string) (string, error) { return m[k], nil }
func (m memSettings) Set(_ context.Context, k, v string) error        { m[k] = v; return nil }

func TestReadMarksOnlyMoveForward(t *testing.T) {
	ctx := context.Background()
	s := NewReadService(memSettings{})
	if _, err := s.Mark(ctx, "m:1", map[string]string{"review": "2026-11-16T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Mark(ctx, "m:1", map[string]string{"review": "2026-11-16T09:00:00Z", "pm": "2026-11-16T11:00:00+08:00"})
	if err != nil {
		t.Fatal(err)
	}
	if got["review"] != "2026-11-16T10:00:00Z" {
		t.Fatalf("an older mark moved it back: %v", got)
	}
	if got["pm"] != "2026-11-16T03:00:00Z" {
		t.Fatalf("marks are kept in UTC: %v", got)
	}
	other, _ := s.Reads(ctx, "m:2")
	if len(other) != 0 {
		t.Fatalf("each viewer has their own marks: %v", other)
	}
	if _, err := s.Mark(ctx, "m:1", map[string]string{"x": "yesterday"}); err == nil {
		t.Fatal("a mark that is not a time is refused")
	}
}
