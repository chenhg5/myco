package oscap

import (
	"context"
)

func screenRecordMissing(*Env) string { return "" }
func recordScreen(ctx context.Context, _ *Env, a screenRecordArgs, path string, emit Emit) error {
	ctx, cancel := screenRecordingContext(ctx)
	defer cancel()
	if err := requestScreenRecordingPermission(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.Interactive {
		emit("selecting", map[string]any{"phase": "region"})
		r, cleanup, err := chooseScreenRegion(ctx, cancel)
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			return err
		}
		if r == nil || r.W < 16 || r.H < 16 {
			return errf(CodeCancelled, "region selection cancelled")
		}
		a.Interactive = false
		a.Region = r
	}
	if ctx.Err() != nil {
		return errf(CodeCancelled, "region selection cancelled")
	}
	return recordNativeMovie(ctx, a, path, emit)
}
