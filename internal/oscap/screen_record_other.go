//go:build !darwin && !windows

package oscap

import "context"

func screenRecordMissing(*Env) string {
	return "use the recording app's browser screen picker on this system"
}
func recordScreen(context.Context, *Env, screenRecordArgs, string, Emit) error {
	return unsupported("screen.record")
}
