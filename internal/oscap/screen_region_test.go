package oscap

import (
	"context"
	"testing"
	"time"
)

func TestScreenRegionSelectionCancelAndMissingProvider(t *testing.T) {
	SetScreenRegionPicker(nil)
	if _, _, err := chooseScreenRegion(context.Background(), nil); err == nil || AsError(err).Code != CodeUnsupported {
		t.Fatalf("missing GUI: %v", err)
	}
	called := false
	SetScreenRegionPicker(func(context.Context, func()) (*ScreenRecordRegion, func(), error) {
		called = true
		return &ScreenRecordRegion{W: 100, H: 100}, func() {}, nil
	})
	defer SetScreenRegionPicker(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := chooseScreenRegion(ctx, nil); err == nil || called {
		t.Fatal("cancelled selection opened a picker")
	}
}
func TestScreenRecordStopCancelsSelection(t *testing.T) {
	input := make(chan []byte, 1)
	ctx, cancel := screenRecordingContext(withInput(context.Background(), input))
	defer cancel()
	input <- []byte("stop\n")
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop during selection was lost")
	}
}
