package oscap

import "sync"

var screenPermission struct {
	sync.RWMutex
	request func() bool
}

// SetScreenRecordingPermission lets the Mac App request permission in its own
// identity before launching a recorder. The CLI keeps its system-tool behaviour.
func SetScreenRecordingPermission(request func() bool) {
	screenPermission.Lock()
	screenPermission.request = request
	screenPermission.Unlock()
}
func requestScreenRecordingPermission() error {
	screenPermission.RLock()
	request := screenPermission.request
	screenPermission.RUnlock()
	if request != nil && !request() {
		return errf(CodePermission, "screen recording is not allowed for this App; open System Settings > Privacy & Security > Screen & System Audio Recording, allow myco-preview or myco, then restart the App if requested")
	}
	return nil
}
