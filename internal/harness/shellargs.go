package harness

import (
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultShell runs shell harnesses and exec nodes: /bin/sh, or PowerShell
// on Windows (present on every Windows 10/11, and it parses its command
// line by the same rules Go quotes with, unlike cmd.exe).
func DefaultShell() string {
	if runtime.GOOS == "windows" {
		return "powershell.exe"
	}
	return "/bin/sh"
}

func shellKind(shell string) string {
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(shell, `\`, "/")))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "powershell", "pwsh":
		return "powershell"
	case "cmd":
		return "cmd"
	}
	return "posix"
}

// IsPOSIXShell reports whether shell takes `-c script [$0 args...]`.
func IsPOSIXShell(shell string) bool { return shellKind(shell) == "posix" }

// ShellArgs are the arguments that make shell run script, followed by args.
// POSIX shells get `-c script args...` (so args[0] is $0); PowerShell gets
// `-NoProfile -NonInteractive -Command script args...`; cmd.exe `/D /C`.
func ShellArgs(shell, script string, args ...string) []string {
	var out []string
	switch shellKind(shell) {
	case "powershell":
		out = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script}
	case "cmd":
		out = []string{"/D", "/C", script}
	default:
		out = []string{"-c", script}
	}
	return append(out, args...)
}
