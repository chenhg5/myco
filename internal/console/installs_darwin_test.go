//go:build darwin

package console

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// The launcher's Info.plist is one macOS reads.
func TestMacBundlePlistIsValid(t *testing.T) {
	path, err := writeMacBundle(t.TempDir(), `报告 & "草稿" <1>`, "h1", "pg_a", []byte("\x89PNG\r\n\x1a\n"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/plutil", "-lint", filepath.Join(path, "Contents", "Info.plist")).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %v %s", err, out)
	}
}
