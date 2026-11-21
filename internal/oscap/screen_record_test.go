package oscap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScreenRecordOptions(t *testing.T) {
	for _, raw := range []string{`{"max_seconds":-1}`, `{"max_seconds":3601}`, `{"max_seconds":"x"}`, `{"region":{"w":0,"h":16}}`, `{"interactive":true,"region":{"w":16,"h":16}}`, `{"region":{"x":32769,"w":16,"h":16}}`} {
		if _, err := decodeScreenRecord(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	a, err := decodeScreenRecord(nil)
	if err != nil || a.MaxSeconds != 600 || a.Microphone {
		t.Fatalf("unsafe defaults: %+v %v", a, err)
	}
}
func TestScreenRecordKeepsOriginal(t *testing.T) {
	home := t.TempDir()
	want := []byte(strings.Repeat("movie", 20))
	v, err := runScreenRecord(context.Background(), &Env{Home: home}, screenRecordArgs{MaxSeconds: 1}, func(string, any) {}, func(_ context.Context, _ screenRecordArgs, p string, _ Emit) error {
		return os.WriteFile(p, want, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	r := v.(screenRecording)
	if r.Bytes != int64(len(want)) || !strings.HasPrefix(r.Path, filepath.Join(home, ".myco", "recordings")+string(filepath.Separator)) {
		t.Fatalf("bad result: %+v", r)
	}
	if b, err := os.ReadFile(r.Path); err != nil || string(b) != string(want) {
		t.Fatalf("original lost: %v", err)
	}
}
func TestScreenRecordCancelCleansEmptyFolder(t *testing.T) {
	home := t.TempDir()
	_, err := runScreenRecord(context.Background(), &Env{Home: home}, screenRecordArgs{MaxSeconds: 1}, func(string, any) {}, func(context.Context, screenRecordArgs, string, Emit) error { return errf(CodeCancelled, "cancelled") })
	if err == nil {
		t.Fatal("cancellation ignored")
	}
	entries, e := os.ReadDir(filepath.Join(home, ".myco", "recordings"))
	if e != nil || len(entries) != 0 {
		t.Fatalf("empty recording left: %v %v", entries, e)
	}
}

func TestScreenRecordExclusiveAndCancelKeepsMovie(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := runScreenRecord(ctx, &Env{Home: home}, screenRecordArgs{MaxSeconds: 1}, func(string, any) {}, func(ctx context.Context, _ screenRecordArgs, path string, _ Emit) error {
			close(entered)
			<-ctx.Done()
			return os.WriteFile(path, []byte(strings.Repeat("movie", 20)), 0600)
		})
		done <- err
	}()
	<-entered
	_, err := runScreenRecord(context.Background(), &Env{Home: home}, screenRecordArgs{MaxSeconds: 1}, func(string, any) {}, func(context.Context, screenRecordArgs, string, Emit) error { t.Error("second capture ran"); return nil })
	if err == nil || AsError(err).Code != CodeExists {
		t.Fatalf("duplicate recording was not rejected: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("finalized movie discarded on cancel: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(home, ".myco", "recordings", "*", "screen.mov"))
	if err != nil || len(files) != 1 {
		t.Fatalf("original missing: %v %v", files, err)
	}
}
