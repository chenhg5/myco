package harness

import (
	"bytes"
	"testing"
)

func TestLineTee(t *testing.T) {
	var buf bytes.Buffer
	var lines []string
	tee := &lineTee{next: &buf, fn: func(b []byte) { lines = append(lines, string(b)) }}
	for _, chunk := range []string{"one\ntw", "o\n", "", "three", "\nfour"} {
		tee.Write([]byte(chunk))
	}
	tee.flush()
	if got := []string{"one", "two", "three", "four"}; len(lines) != 4 || lines[0] != got[0] || lines[1] != got[1] || lines[2] != got[2] || lines[3] != got[3] {
		t.Fatalf("lines %q", lines)
	}
	if buf.String() != "one\ntwo\nthree\nfour" {
		t.Fatalf("passed through %q", buf.String())
	}
}
