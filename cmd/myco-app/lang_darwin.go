package main

import (
	"os"
	"os/exec"
	"strings"
)

// systemPrefersChinese reports whether the first of the user's preferred
// languages (System Settings → General → Language & Region) is Chinese.
func systemPrefersChinese() bool {
	if out, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			l = strings.Trim(strings.TrimSpace(l), `",()`)
			if l != "" {
				return strings.HasPrefix(strings.ToLower(l), "zh")
			}
		}
	}
	return strings.HasPrefix(strings.ToLower(os.Getenv("LANG")), "zh")
}
