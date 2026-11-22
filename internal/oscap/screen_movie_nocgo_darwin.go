//go:build darwin && !cgo

package oscap

import "context"

func recordNativeMovie(context.Context, screenRecordArgs, string, Emit) error {
	return errf(CodeUnsupported, "screen recording needs the native myco desktop App")
}
