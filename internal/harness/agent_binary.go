package harness

import (
	"os"
	"os/exec"
)

// ResolveAgentBinary preserves explicit overrides and PATH precedence. CodeBuddy
// is also shipped inside WorkBuddy, without a PATH entry on desktop installs.
func ResolveAgentBinary(binary string) string {
	if binary != "codebuddy" {
		return binary
	}
	if p, err := exec.LookPath(binary); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range WorkBuddyBinaries(home) {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && bundledExecutable(st) {
			return p
		}
	}
	return binary
}
