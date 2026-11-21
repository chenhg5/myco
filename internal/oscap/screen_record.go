package oscap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Screen recording is separate from remote desktop: it never starts a remote
// connection, loads a codec component or sends footage to the workspace.
type screenRecordRegion struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}
type screenRecordArgs struct {
	Region      *screenRecordRegion `json:"region,omitempty"`
	Interactive bool                `json:"interactive"`
	Microphone  bool                `json:"microphone"`
	Clicks      bool                `json:"clicks"`
	MaxSeconds  int                 `json:"max_seconds"`
}
type screenRecording struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

var screenRecordMu sync.Mutex

func init() {
	register(&Cap{Name: "screen.record", Job: true, About: "Record this screen locally; write stop to finish the movie", Missing: screenRecordMissing, Fn: screenRecord})
}
func decodeScreenRecord(raw json.RawMessage) (screenRecordArgs, error) {
	var a screenRecordArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return a, errf(CodeInvalid, "invalid screen recording options")
		}
	}
	if a.MaxSeconds == 0 {
		a.MaxSeconds = 600
	}
	if a.MaxSeconds < 1 || a.MaxSeconds > 3600 {
		return a, errf(CodeInvalid, "max_seconds must be between 1 and 3600")
	}
	if r := a.Region; r != nil {
		if a.Interactive || r.W < 16 || r.H < 16 || r.W > 16384 || r.H > 16384 || r.X < -32768 || r.X > 32768 || r.Y < -32768 || r.Y > 32768 {
			return a, errf(CodeInvalid, "region needs non-interactive capture and a valid rectangle (16–16384 pixels per side)")
		}
	}
	return a, nil
}
func screenRecord(ctx context.Context, env *Env, raw json.RawMessage, emit Emit) (any, error) {
	a, err := decodeScreenRecord(raw)
	if err != nil {
		return nil, err
	}
	return runScreenRecord(ctx, env, a, emit, func(ctx context.Context, a screenRecordArgs, path string, emit Emit) error {
		return recordScreen(ctx, env, a, path, emit)
	})
}
func runScreenRecord(ctx context.Context, env *Env, a screenRecordArgs, emit Emit, capture func(context.Context, screenRecordArgs, string, Emit) error) (any, error) {
	var err error
	if !screenRecordMu.TryLock() {
		return nil, errf(CodeExists, "a screen recording is already running")
	}
	defer screenRecordMu.Unlock()
	if env == nil || env.Home == "" {
		return nil, errf(CodeFailed, "home folder is unavailable")
	}
	root := filepath.Join(env.Home, ".myco", "recordings")
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, "screen-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "screen.mov")
	// Keep the original on disk even if importing it into the editor fails.
	defer func() { _ = os.Remove(dir) }() // removes only an empty, cancelled recording
	limit, cancel := context.WithTimeout(ctx, time.Duration(a.MaxSeconds+10)*time.Second)
	defer cancel()
	if err = capture(limit, a, path, emit); err != nil {
		return nil, err
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() < 32 {
		return nil, errf(CodeCancelled, "recording cancelled or no movie was produced")
	}
	return screenRecording{Path: path, Bytes: fi.Size()}, nil
}
