package main

import (
	"context"
	"testing"
	"time"
)

func TestScreenRegionCoordinates(t *testing.T) {
	for _, c := range []struct {
		x, y, w, h, top            float64
		wantX, wantY, wantW, wantH int
	}{
		{20, 100, 400, 300, 900, 20, 500, 400, 300},
		{-1800, 1000, 100, 100, 900, -1800, -200, 100, 100},
		{.8, 100.2, 17.8, 20.3, 900, 0, 779, 19, 21},
	} {
		r := screenRegionFromCocoa(c.x, c.y, c.w, c.h, c.top)
		if r == nil || r.X != c.wantX || r.Y != c.wantY || r.W != c.wantW || r.H != c.wantH {
			t.Fatalf("%+v -> %+v", c, r)
		}
	}
	if screenRegionFromCocoa(0, 0, 0, 100, 900) != nil {
		t.Fatal("empty selection accepted")
	}
}

func TestScreenCountdownCompletesBeforeCapture(t *testing.T) {
	var seen []int
	started := time.Now()
	if err := screenCountdown(context.Background(), 5, 5*time.Millisecond, func(n int) { seen = append(seen, n) }); err != nil {
		t.Fatal(err)
	}
	if seen[0] != 5 || seen[len(seen)-1] != 0 || time.Since(started) < 25*time.Millisecond {
		t.Fatalf("countdown: %v", seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] > seen[i-1] {
			t.Fatalf("countdown went backwards: %v", seen)
		}
	}
}
func TestScreenCountdownCancelDoesNotStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var seen []int
	err := screenCountdown(ctx, 5, time.Millisecond, func(n int) {
		seen = append(seen, n)
		if n == 5 {
			cancel()
		}
	})
	if err == nil || seen[len(seen)-1] == 0 {
		t.Fatalf("cancelled countdown started capture: %v %v", seen, err)
	}
	cancel()
	if err := screenCountdown(ctx, 5, time.Millisecond, func(int) { t.Fatal("cancelled countdown updated UI") }); err == nil {
		t.Fatal("expected cancellation")
	}
}
