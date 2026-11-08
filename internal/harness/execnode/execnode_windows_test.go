//go:build windows

package execnode

import (
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// The default shell on Windows is PowerShell: exit codes, stdout and stderr,
// a clean environment and timeouts must work through it.
func TestExecPowerShell(t *testing.T) {
	spec := harness.ExecSpec{Senders: []string{"boss"}}
	start := time.Now()
	res := run(t, spec, "boss", "Write-Output out; [Console]::Error.WriteLine('err'); exit 3")
	t.Logf("first PowerShell command took %s", time.Since(start))
	if res.ExitCode != 3 || strings.TrimSpace(res.Stdout) != "out" || strings.TrimSpace(res.Stderr) != "err" || res.Denied {
		t.Fatalf("%+v", res)
	}
	start = time.Now()
	if res := run(t, spec, "boss", "echo ok"); strings.TrimSpace(res.Stdout) != "ok" {
		t.Fatalf("echo: %+v", res)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("echo ok took %s: PowerShell is missing something from the clean environment", d)
	} else {
		t.Logf("echo ok took %s", d)
	}
}

func TestExecPowerShellEnvironmentIsClean(t *testing.T) {
	t.Setenv("MYCO_TOKEN", "secret-from-runner-env")
	t.Setenv("SOME_RUNNER_SECRET", "x")
	res := run(t, harness.ExecSpec{Senders: []string{"boss"}}, "boss", "Get-ChildItem env: | ForEach-Object { $_.Name + '=' + $_.Value }",
		"MYCO_TOKEN=secret-passed-in", "MYCO_CONFIG=/x", "MYCO_FROM=boss")
	if strings.Contains(res.Stdout, "secret") || strings.Contains(res.Stdout, "SOME_RUNNER_SECRET") || strings.Contains(res.Stdout, "MYCO_CONFIG") {
		t.Fatalf("credentials leaked into the command env:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "MYCO_FROM=boss") || !strings.Contains(strings.ToUpper(res.Stdout), "SYSTEMROOT=") {
		t.Fatalf("expected context and Windows basics:\n%s", res.Stdout)
	}
}

// Timeouts and the output cap through PowerShell (generous limits: it can
// take a second or two to start on a busy machine).
func TestExecPowerShellTimeoutAndTruncation(t *testing.T) {
	spec := harness.ExecSpec{Senders: []string{"boss"}, Timeout: "3s"}
	start := time.Now()
	if res := run(t, spec, "boss", "Start-Sleep -Seconds 60"); !res.TimedOut || res.ExitCode != ExitTimedOut {
		t.Fatalf("timeout: %+v", res)
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("a timed-out command took %s to be killed", d)
	}
	spec = harness.ExecSpec{Senders: []string{"boss"}, MaxOutput: 10}
	if res := run(t, spec, "boss", "Write-Output '0123456789abcdef'"); !res.Truncated || res.Stdout != "0123456789" {
		t.Fatalf("truncation: %+v", res)
	}
}
