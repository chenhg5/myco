package requestid

import (
	"context"
	"testing"
)

func TestValidAndContext(t *testing.T) {
	for id, ok := range map[string]bool{"": false, "abc-123_x.y": true, "a b": false, "x\nlevel=ERROR": false, string(make([]byte, 65)): false} {
		if (Valid(id) != "") != ok {
			t.Errorf("Valid(%q): %v", id, !ok)
		}
	}
	if n := New(); len(n) != 16 || Valid(n) != n || New() == n {
		t.Fatalf("New: %q", n)
	}
	if From(With(context.Background(), "abc")) != "abc" || From(context.Background()) != "" {
		t.Fatal("context")
	}
}
