package oscap

import "testing"

func TestScreenRecordingPermission(t *testing.T) {
	defer SetScreenRecordingPermission(nil)
	SetScreenRecordingPermission(func() bool { return false })
	if err := requestScreenRecordingPermission(); err == nil || AsError(err).Code != CodePermission {
		t.Fatalf("permission denial lost: %v", err)
	}
	SetScreenRecordingPermission(func() bool { return true })
	if err := requestScreenRecordingPermission(); err != nil {
		t.Fatal(err)
	}
	SetScreenRecordingPermission(nil)
	if err := requestScreenRecordingPermission(); err != nil {
		t.Fatalf("CLI behaviour changed: %v", err)
	}
}
