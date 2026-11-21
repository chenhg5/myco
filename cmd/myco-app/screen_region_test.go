package main

import "testing"

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
