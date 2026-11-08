// Package machineid identifies this machine for the hub: a hash of the
// operating system's own stable machine ID, so the same device is the same
// machine however often it joins or renames itself. The raw ID never
// leaves the machine.
package machineid

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// Fingerprint returns the hashed machine ID, or a hash of the host name
// when the OS ID cannot be read.
func Fingerprint() string { return hash(raw()) }

// Cloud returns the fingerprint of a cloud environment (claude.ai sessions
// share one environment name per environment, and containers come and go).
func Cloud(environment string) string { return hash("cloud:" + environment) }

func hash(id string) string {
	sum := sha256.Sum256([]byte("myco-machine:" + id))
	return hex.EncodeToString(sum[:16])
}

var platformUUID = regexp.MustCompile(`"IOPlatformUUID" = "([^"]+)"`)

func raw() string {
	switch runtime.GOOS {
	case "darwin":
		if out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output(); err == nil {
			if m := platformUUID.FindSubmatch(out); m != nil {
				return "darwin:" + string(m[1])
			}
		}
	case "linux":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(b)) > 0 {
				return "linux:" + string(bytes.TrimSpace(b))
			}
		}
	case "windows":
		if out, err := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if f := strings.Fields(line); len(f) == 3 && f[0] == "MachineGuid" {
					return "windows:" + f[2]
				}
			}
		}
	}
	h, _ := os.Hostname()
	return "host:" + strings.ToLower(h)
}
