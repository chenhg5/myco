package diskspace

import "testing"

func TestReserveAccountsForPendingUpload(t *testing.T) {
	for _, tc := range []struct {
		name          string
		s             Space
		incoming, min int64
		percent       int
		allowed       bool
	}{
		{"absolute", Space{100, 1000}, 0, 100, 5, true}, {"below absolute", Space{99, 1000}, 0, 100, 5, false},
		{"pending write", Space{110, 1000}, 11, 100, 5, false}, {"exact remaining", Space{110, 1000}, 10, 100, 5, true},
		{"percentage", Space{499, 10000}, 0, 100, 5, false}, {"percentage boundary", Space{500, 10000}, 0, 100, 5, true},
		{"no underflow", Space{0, 10000}, 1, 100, 5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := Check(tc.s, tc.incoming, tc.min, tc.percent); (e == nil) != tc.allowed {
				t.Fatalf("allowed=%v error=%v", tc.allowed, e)
			}
		})
	}
}
func TestNativeSpace(t *testing.T) {
	s, e := Read(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if s.Total == 0 || s.Available > s.Total {
		t.Fatalf("invalid disk space %+v", s)
	}
}
