package harness

import (
	"reflect"
	"testing"
)

func TestShellArgs(t *testing.T) {
	for _, c := range []struct {
		shell string
		want  []string
	}{
		{"/bin/sh", []string{"-c", "echo ok", "x"}},
		{"bash", []string{"-c", "echo ok", "x"}},
		{"powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", "echo ok", "x"}},
		{`C:\Program Files\PowerShell\7\pwsh.exe`, []string{"-NoProfile", "-NonInteractive", "-Command", "echo ok", "x"}},
		{`C:\Windows\System32\CMD.EXE`, []string{"/D", "/C", "echo ok", "x"}},
	} {
		if got := ShellArgs(c.shell, "echo ok", "x"); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ShellArgs(%q) = %q, want %q", c.shell, got, c.want)
		}
	}
	if !IsPOSIXShell("/bin/bash") || IsPOSIXShell("pwsh") {
		t.Error("IsPOSIXShell")
	}
}
