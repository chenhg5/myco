package main

import (
	"bytes"
	"fmt"
	"runtime/pprof"
	"sync"
	"testing"
	"time"
)

// Metadata only: never record CDP arguments, page text or credentials.
func nativeDiagnostics(t *testing.T, e *engine) func() {
	t.Helper()
	var mu sync.Mutex
	var rows []string
	start := time.Now()
	e.trace = func(sid, method string, done bool) {
		mu.Lock()
		defer mu.Unlock()
		if len(sid) > 6 {
			sid = sid[:6]
		}
		rows = append(rows, fmt.Sprintf("%s sid=%s %s done=%v", time.Since(start).Round(time.Millisecond), sid, method, done))
		if len(rows) > 256 {
			rows = rows[len(rows)-256:]
		}
	}
	return func() {
		t.Helper()
		mu.Lock()
		copyRows := append([]string{}, rows...)
		mu.Unlock()
		for _, row := range copyRows {
			t.Log(row)
		}
		var stack bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stack, 2)
		t.Logf("goroutines:\n%s", stack.String())
	}
}
