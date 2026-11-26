package main

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-rod/rod/lib/cdp"
)

// This separate logger accepts static event names, concrete error types and
// numeric CDP codes only. Never write error messages, page text, URLs or args.
var controlLog = log.New(os.Stderr, "myco-browser diagnostic: ", log.LstdFlags)

func controlDiagnostic(event string, err error) {
	code := 0
	var native *cdp.Error
	if errors.As(err, &native) {
		code = native.Code
	}
	controlLog.Printf("event=%s error_type=%T cdp_code=%d", event, err, code)
}

type controlLogWriter struct {
	mu    sync.Mutex
	file  *os.File
	bytes int64
}

const controlLogMax = 1 << 20

func (w *controlLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if int64(len(p)) > controlLogMax {
		return len(p), nil
	}
	if w.bytes+int64(len(p)) > controlLogMax {
		if err := w.file.Truncate(0); err != nil {
			return 0, err
		}
		if _, err := w.file.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
		w.bytes = 0
	}
	n, err := w.file.Write(p)
	w.bytes += int64(n)
	return n, err
}
func openControlDiagnostic(dir string) func() {
	path := filepath.Join(dir, "component-diagnostic.log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() >= controlLogMax {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return func() {}
	}
	if file.Chmod(0600) != nil {
		file.Close()
		return func() {}
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return func() {}
	}
	previous := controlLog.Writer()
	controlLog.SetOutput(&controlLogWriter{file: file, bytes: info.Size()})
	return func() { controlLog.SetOutput(previous); file.Close() }
}

var _ io.Writer = (*controlLogWriter)(nil)
