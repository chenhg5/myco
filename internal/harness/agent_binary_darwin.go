package harness

import (
	"os"
	"path/filepath"
)

func WorkBuddyBinaries(home string) []string {
	suffix := filepath.Join("WorkBuddy.app", "Contents", "Resources", "app.asar.unpacked", "cli", "bin", "codebuddy")
	return []string{filepath.Join(home, "Applications", suffix), filepath.Join("/Applications", suffix)}
}
func bundledExecutable(st os.FileInfo) bool { return st.Mode()&0o111 != 0 }
