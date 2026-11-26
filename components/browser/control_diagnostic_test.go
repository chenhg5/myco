package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

func TestControlDiagnosticsNeverStoreNativeMessages(t *testing.T) {
	dir := t.TempDir()
	closeLog := openControlDiagnostic(dir)
	controlNativeError(&cdp.Error{Code: -32000, Message: "private-page-text", Data: "private-native-data"})
	controlNativeError(errors.New("https://private.example/?token=private-value"))
	closeLog()
	path := filepath.Join(dir, "component-diagnostic.log")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "control_error_fallback") || !strings.Contains(string(data), "cdp_code=-32000") {
		t.Fatal(string(data), err)
	}
	for _, value := range []string{"private-page", "private-native", "private.example", "token="} {
		if strings.Contains(string(data), value) {
			t.Fatal("native detail entered diagnostic log")
		}
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("diagnostics not private")
	}
}
func TestDestroyedCollectorContextIsRemoved(t *testing.T) {
	session, _, send := navigationFixture(t)
	send(&proto.RuntimeExecutionContextDestroyed{ExecutionContextID: 1})
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.contexts["sid"]) != 0 || len(session.contexts["oopif-sid"]) != 1 {
		t.Fatal("destroyed context retained or unrelated frame lost", session.contexts)
	}
}

func TestControlDiagnosticsKeepRecentEntriesAtLimit(t *testing.T) {
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "diagnostic.log"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := &controlLogWriter{file: file}
	if _, err = writer.Write([]byte(strings.Repeat("x", controlLogMax-2))); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write([]byte("recent diagnostic\n")); err != nil {
		t.Fatal(err)
	}
	info, _ := file.Stat()
	raw, err := os.ReadFile(file.Name())
	if err != nil || info.Size() > controlLogMax || string(raw) != "recent diagnostic\n" {
		t.Fatal("log limit discarded all future diagnostics", info.Size(), err)
	}
}

func TestDelayedDestroyedContextCannotRemoveReusedID(t *testing.T) {
	session, _, send := navigationFixture(t)
	session.contextKeys = map[string]map[proto.RuntimeExecutionContextID]string{"sid": {1: "current-world"}}
	send(&proto.RuntimeExecutionContextDestroyed{ExecutionContextID: 1, ExecutionContextUniqueID: "old-world"})
	session.mu.Lock()
	count := len(session.contexts["sid"])
	session.mu.Unlock()
	if count != 1 {
		t.Fatal("delayed destruction removed reused context")
	}
	send(&proto.RuntimeExecutionContextDestroyed{ExecutionContextID: 1, ExecutionContextUniqueID: "current-world"})
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.contexts["sid"]) != 0 || len(session.contextKeys["sid"]) != 0 {
		t.Fatal("live destruction not cleaned")
	}
}
