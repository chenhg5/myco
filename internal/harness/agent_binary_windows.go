package harness

import (
	"os"
	"path/filepath"
)

func WorkBuddyBinaries(home string) []string {
	var out []string
	for _, root := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if root == "" {
			continue
		}
		for _, base := range []string{root, filepath.Join(root, "Programs")} {
			out = append(out, filepath.Join(base, "WorkBuddy", "resources", "app.asar.unpacked", "cli", "bin", "codebuddy.cmd"))
		}
	}
	return out
}
func bundledExecutable(st os.FileInfo) bool { return true }
