package service

import (
	"strings"
	"testing"
)

func TestWindowsLogonPlan(t *testing.T) {
	w := &WindowsLogon{Home: `C:\Users\Glenn O'Neil`, R: &fakeRunner{}}
	sp := Spec{Binary: `C:\Users\Glenn O'Neil\AppData\Local\Programs\myco\myco.exe`, Args: []string{"runner", "start", "--all", "--max-concurrent", "3"},
		Env: map[string]string{"PATH": `C:\Program Files\nodejs;C:\Windows`}, LogPath: `C:\Users\Glenn O'Neil\.myco\runner.log`}
	p, err := w.PlanInstall(sp)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`$env:PATH = 'C:\Program Files\nodejs;C:\Windows'`,
		`Start-Process -WindowStyle Hidden -FilePath 'C:\Users\Glenn O''Neil\AppData\Local\Programs\myco\myco.exe'`,
		`-ArgumentList 'runner start --all --max-concurrent 3 --log-file "C:\Users\Glenn O''Neil\.myco\runner.log"'`,
		`if (Get-CimInstance Win32_Process -Filter "Name='myco.exe'" | Where-Object { $_.CommandLine -like '*runner start --all*' }) { exit 0 }`,
	} {
		if !strings.Contains(p.Content, want) {
			t.Fatalf("launcher lacks %q:\n%s", want, p.Content)
		}
	}
	if !strings.HasSuffix(p.File, `ai.myco.runner.ps1`) {
		t.Fatalf("file %s", p.File)
	}
	reg := p.Commands[0]
	if reg[0] != "reg" || reg[2] != RunKey || reg[4] != "ai.myco.runner" || !strings.Contains(reg[8], "-WindowStyle Hidden") || !strings.Contains(reg[8], `-File "`+p.File+`"`) {
		t.Fatalf("run key command %q", reg)
	}
	if p.Commands[1][0] != "-powershell.exe" || p.Commands[2][len(p.Commands[2])-1] != p.File {
		t.Fatalf("commands %q", p.Commands)
	}
}

func TestWinArg(t *testing.T) {
	for in, want := range map[string]string{
		"plain":           "plain",
		"two words":       `"two words"`,
		`C:\dir with sp\`: `"C:\dir with sp\\"`,
		`say "hi"`:        `"say \"hi\""`,
		"":                `""`,
	} {
		if got := winArg(in); got != want {
			t.Errorf("winArg(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestWindowsLogonStatus(t *testing.T) {
	f := &fakeRunner{output: map[string]string{}}
	w := &WindowsLogon{Home: t.TempDir(), R: f}
	find := powershell(findProcess + " | Select-Object -First 1 -ExpandProperty ProcessId")
	f.output[strings.Join(find, " ")] = "4242\r\n"
	st, err := w.Status()
	if err != nil || !st.Running || st.PID != 4242 {
		t.Fatalf("status %+v %v", st, err)
	}
}
