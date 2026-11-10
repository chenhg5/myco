//go:build windows

package httpapi_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/app"
)

// parsePowerShell reports PowerShell's syntax errors in a script file.
func parsePowerShell(t *testing.T, file string) string {
	t.Helper()
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("no powershell.exe")
	}
	script := `$e = $null; [void][System.Management.Automation.Language.Parser]::ParseFile('` + strings.ReplaceAll(file, "'", "''") + `', [ref]$null, [ref]$e); $e | ForEach-Object { $_.Message }`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("powershell: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// The PowerShell join script and installer parse in Windows PowerShell.
func TestPowerShellScriptsParse(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	tok, _, _ := h.createLink(map[string]any{"max_uses": 1})
	join := h.do("GET", "/j/"+tok+"?format=ps1", "", nil).want(t, 200).raw
	bad := h.do("GET", "/j/nope?format=ps1", "", nil).want(t, 200).raw
	dir := t.TempDir()
	for name, body := range map[string][]byte{"join.ps1": join, "bad.ps1": bad} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if errs := parsePowerShell(t, p); errs != "" {
			t.Errorf("%s does not parse: %s\n%s", name, errs, body)
		}
	}
	if errs := parsePowerShell(t, filepath.Join("..", "..", "..", "deploy", "install-client.ps1")); errs != "" {
		t.Errorf("install-client.ps1 does not parse: %s", errs)
	}
}
