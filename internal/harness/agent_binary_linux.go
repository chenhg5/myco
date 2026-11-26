package harness

import (
	"os"
	"path/filepath"
)

// The official WorkBuddy Linux FAQ identifies /opt/apps/workbuddy/workbuddy.
// Probe its unpacked Electron resources, not third-party repackaging paths.
func WorkBuddyBinaries(home string) []string {
	return []string{filepath.Join("/opt/apps/workbuddy", "resources", "app.asar.unpacked", "cli", "bin", "codebuddy")}
}
func bundledExecutable(st os.FileInfo) bool { return st.Mode()&0o111 != 0 }
