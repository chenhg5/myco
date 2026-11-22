package console

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTailscaleInstallFoundWithoutNewPATH(t *testing.T) {
	programs := t.TempDir()
	bin := filepath.Join(programs, "Tailscale", "tailscale.exe")
	if err := os.MkdirAll(filepath.Dir(bin), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("test executable"), 0755); err != nil {
		t.Fatal(err)
	}
	paths := windowsTailscalePaths(func(key string) string {
		if key == "ProgramFiles" {
			return programs
		}
		return ""
	})
	local := &Local{Home: t.TempDir(), Dirs: []string{}, LookPath: func(string) (string, error) { return "", errors.New("old PATH") }}
	if got := local.find("tailscale", paths...); got != bin {
		t.Fatalf("new installation not found: %s", got)
	}
	if len(windowsTailscalePaths(func(string) string { return "" })) != 1 {
		t.Fatal("missing default installer location")
	}
}
