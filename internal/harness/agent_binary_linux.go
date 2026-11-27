package harness

import (
	"os"
	"path/filepath"
)

// The official deb installs under /opt/WorkBuddy (case-sensitive). Keep the
// /opt/apps/workbuddy layout documented for UOS/Kylin store installations too.
func WorkBuddyBinaries(home string) []string {
	return []string{
		filepath.Join("/opt/WorkBuddy", "resources", "app.asar.unpacked", "cli", "bin", "codebuddy"),
		filepath.Join("/opt/apps/workbuddy", "resources", "app.asar.unpacked", "cli", "bin", "codebuddy"),
	}
}
func bundledExecutable(st os.FileInfo) bool { return st.Mode()&0o111 != 0 }
