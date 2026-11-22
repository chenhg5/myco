package console

import "path/filepath"

// An App opened before installation keeps its old PATH. Check the installer's
// standard locations too, so Check again works without requiring a new login.
func windowsTailscalePaths(getenv func(string) string) []string {
	var paths []string
	for _, key := range []string{"ProgramW6432", "ProgramFiles", "ProgramFiles(x86)"} {
		if root := getenv(key); root != "" {
			paths = append(paths, filepath.Join(root, "Tailscale", "tailscale.exe"))
		}
	}
	if len(paths) == 0 {
		paths = append(paths, filepath.Join(`C:\Program Files`, "Tailscale", "tailscale.exe"))
	}
	return paths
}
