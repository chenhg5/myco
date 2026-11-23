package console

import (
	"reflect"
	"testing"
)

func TestParseShellEnv(t *testing.T) {
	out := []byte("Welcome to zsh!\nsome rc output\n__MYCO_ENV__\nPATH=/usr/bin\x00KEY=a=b\x00MULTI=line1\nline2\x00=bad\x00")
	want := []string{"PATH=/usr/bin", "KEY=a=b", "MULTI=line1\nline2"}
	if got := parseShellEnv(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	if got := parseShellEnv([]byte("no marker")); got != nil {
		t.Fatalf("no marker: %q", got)
	}
}

func TestUserEnvPrefersTheShell(t *testing.T) {
	t.Setenv("MYCO_TEST_APP_ONLY", "app")
	t.Setenv("MYCO_TEST_BOTH", "app")
	l := &Local{ShellEnv: func() []string { return []string{"MYCO_TEST_BOTH=shell", "MYCO_TEST_SHELL_ONLY=s"} }}
	for k, want := range map[string]string{"MYCO_TEST_APP_ONLY": "app", "MYCO_TEST_BOTH": "shell", "MYCO_TEST_SHELL_ONLY": "s"} {
		if got := l.userGetenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if got := (&Local{}).userGetenv("MYCO_TEST_BOTH"); got != "app" {
		t.Errorf("no shell: %q", got)
	}
}
