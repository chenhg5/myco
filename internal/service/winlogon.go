package service

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// WindowsLogon runs the runner on Windows: a value under the user's Run key
// starts it hidden at every logon (no admin rights needed), and installing
// starts it now. A small PowerShell launcher holds the command and its
// environment, and does nothing when the runner is already running.
type WindowsLogon struct {
	Home string
	R    Runner
	ID   ID
}

// RunKey is where Windows keeps the user's logon commands.
const RunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// winMatch picks the runner's process out of every myco.exe.
const winMatch = "runner start --all"

func (w *WindowsLogon) Name() string { return "windows-logon" }

func (w *WindowsLogon) value() string { return w.ID.orDefault().Label }

func (w *WindowsLogon) file() string {
	return filepath.Join(w.Home, ".myco", w.value()+".ps1")
}

// psq quotes s for PowerShell (single quotes).
func psq(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// winArg quotes one argument for a Windows command line.
func winArg(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\"") {
		return a
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range a {
		switch r {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, 2*slashes+1))
			slashes = 0
		default:
			slashes = 0
		}
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, slashes))
	b.WriteByte('"')
	return b.String()
}

// Launcher renders the PowerShell script that starts the runner hidden.
func (w *WindowsLogon) Launcher(sp Spec) string {
	args := append(append([]string{}, sp.Args...), "--log-file", sp.LogPath)
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = winArg(a)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: started hidden at logon by %s (value %s).\n", w.ID.orDefault().Description, RunKey, w.value())
	b.WriteString("$ErrorActionPreference = 'SilentlyContinue'\n")
	for _, k := range sortedKeys(sp.Env) {
		fmt.Fprintf(&b, "$env:%s = %s\n", k, psq(sp.Env[k]))
	}
	fmt.Fprintf(&b, "if (%s) { exit 0 }\n", findProcess)
	fmt.Fprintf(&b, "Start-Process -WindowStyle Hidden -FilePath %s -ArgumentList %s\n", psq(sp.Binary), psq(strings.Join(quoted, " ")))
	return b.String()
}

// findProcess lists the runner's processes (PowerShell).
var findProcess = `Get-CimInstance Win32_Process -Filter "Name='myco.exe'" | Where-Object { $_.CommandLine -like '*` + winMatch + `*' }`

func powershell(script string) []string {
	return []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script}
}

func (w *WindowsLogon) launch() []string {
	return []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", w.file()}
}

func (w *WindowsLogon) stop() []string {
	return powershell(findProcess + " | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }")
}

func (w *WindowsLogon) PlanInstall(sp Spec) (Plan, error) {
	atLogon := `powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "` + w.file() + `"`
	stop := w.stop()
	stop[0] = "-" + stop[0] // nothing to stop on a first install
	return Plan{
		Manager: w.Name(), File: w.file(), Content: w.Launcher(sp),
		Commands: [][]string{
			{"reg", "add", RunKey, "/v", w.value(), "/t", "REG_SZ", "/d", atLogon, "/f"},
			stop,
			w.launch(),
		},
		Notes: []string{"starts now, hidden, and at every logon (no admin rights needed); it stops when you log out or shut down",
			"log: " + sp.LogPath},
	}, nil
}

func (w *WindowsLogon) PlanUninstall() Plan {
	stop := w.stop()
	stop[0] = "-" + stop[0]
	return Plan{Manager: w.Name(), File: w.file(), Commands: [][]string{
		{"-reg", "delete", RunKey, "/v", w.value(), "/f"},
		stop,
	}}
}

func (w *WindowsLogon) Status() (Status, error) {
	st := Status{Manager: w.Name(), File: w.file()}
	if _, err := w.R.Run("reg", "query", RunKey, "/v", w.value()); err == nil {
		st.Installed = exists(w.file())
	}
	find := powershell(findProcess + " | Select-Object -First 1 -ExpandProperty ProcessId")
	out, err := w.R.Run(find[0], find[1:]...)
	if err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(out)); perr == nil && pid > 0 {
			st.Running, st.PID = true, pid
		}
	}
	if !st.Running {
		st.Detail = "not running"
	}
	return st, nil
}

func (w *WindowsLogon) Restart() error {
	stop := w.stop()
	if _, err := w.R.Run(stop[0], stop[1:]...); err != nil {
		return err
	}
	launch := w.launch()
	_, err := w.R.Run(launch[0], launch[1:]...)
	return err
}
