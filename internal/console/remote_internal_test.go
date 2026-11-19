package console

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestSSHConfigNames(t *testing.T) {
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(filepath.Join(ssh, "conf.d"), 0o700)
	_ = os.WriteFile(filepath.Join(ssh, "config"), []byte(`# mine
Host *
  ServerAliveInterval 30
Host vps vps-alias
  HostName 1.2.3.4
Host="dev box" devbox
Include conf.d/*.conf
Include ~/.ssh/config
Host github.com
  User git
Host !bad *.corp
`), 0o600)
	_ = os.WriteFile(filepath.Join(ssh, "conf.d", "a.conf"), []byte("Host wsl\n  Port 2222\nHost vps\n"), 0o600)
	got := strings.Join(sshConfigNames(home), ",")
	if got != "vps,devbox,wsl,github.com" {
		t.Fatalf("names: %s", got)
	}
}

func TestParseSSHG(t *testing.T) {
	h := parseSSHG("vps", "user root\nhostname 1.2.3.4\nport 2222\nproxyjump none\nidentityfile ~/.ssh/id_ed25519\n")
	if h.User != "root" || h.Host != "1.2.3.4" || h.Port != 2222 || h.Jump != "" {
		t.Fatalf("%+v", h)
	}
	if h := parseSSHG("in", "hostname 10.0.0.2\nproxyjump bastion\n"); h.Jump != "bastion" {
		t.Fatalf("%+v", h)
	}
}

func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

// wsl -l -v prints UTF-16 with a header in the system's language.
func TestParseWSLList(t *testing.T) {
	raw := utf16le("  名称            状态           版本\r\n* Ubuntu-22.04    Running         2\r\n  Debian          Stopped         2\r\n  docker-desktop  Stopped         2\r\n")
	ds := parseWSLList(raw)
	if len(ds) != 2 || ds[0].Name != "Ubuntu-22.04" || !ds[0].Default || !ds[0].Running || ds[1].Name != "Debian" || ds[1].Running || ds[1].Version != 2 {
		t.Fatalf("%+v", ds)
	}
}

func TestClassifySSH(t *testing.T) {
	rt := Route{Via: "ssh", Target: "root@1.2.3.4", Port: 2222}
	for stderr, want := range map[string]string{
		"root@1.2.3.4: Permission denied (publickey,password).":                                                                  "auth",
		"No ED25519 host key is known for [1.2.3.4]:2222 and you have requested strict checking.\nHost key verification failed.": "host_unknown",
		"@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@\nHost key verification failed.":                                "host_changed",
		"ssh: connect to host 1.2.3.4 port 2222: Connection refused":                                                             "refused",
		"ssh: Could not resolve hostname nope: Name or service not known":                                                        "unknown_host",
		"ssh: connect to host 1.2.3.4 port 2222: Operation timed out":                                                            "unreachable",
	} {
		e := classifySSH(stderr, rt)
		if e.Code != want || e.Command != "ssh -p 2222 root@1.2.3.4" {
			t.Fatalf("%q: %+v, want %s", stderr, e, want)
		}
	}
}

func TestRouteCommand(t *testing.T) {
	name, args := Route{Via: "ssh", Target: "vps"}.command()
	j := strings.Join(args, " ")
	if name != "ssh" || !strings.Contains(j, "BatchMode=yes") || !strings.HasSuffix(j, "-- vps sh -s") {
		t.Fatalf("%s %s", name, j)
	}
	name, args = Route{Via: "wsl", Target: "Ubuntu"}.command()
	if name != "wsl.exe" || strings.Join(args, " ") != "-d Ubuntu --cd ~ -- sh -s" {
		t.Fatalf("%s %v", name, args)
	}
}

// The join link reaches curl on its stdin, not on a command line.
func TestInstallScriptKeepsTheLinkOffCommandLines(t *testing.T) {
	s := installScript("https://hub.example/join/tok'en")
	if !strings.Contains(s, `printf 'url = "%s"\n' 'https://hub.example/join/tok'\''en' | curl -fsSL --retry 3 -K - 2>&1)"`) {
		t.Fatalf("script:\n%s", s)
	}
	if p := parseProbe([]string{"noise", "myco-probe os Linux", "myco-probe myco v0.20.17", "myco-probe end ok"}); p["os"] != "Linux" || p["myco"] != "v0.20.17" || p["end"] != "ok" {
		t.Fatalf("%v", p)
	}
}

func TestParseDiagMasksSecrets(t *testing.T) {
	found, hubs, log := parseDiag([]string{
		"myco-check myco v0.20.17",
		"myco-check service active",
		"myco-check proxy http://alice:s3cret@127.0.0.1:7890",
		"myco-check hub https://hub.example.com",
		"myco-check hub_direct https://hub.example.com 200",
		"myco-check hub_proxy https://hub.example.com 000",
		"myco-log time=x level=WARN msg=\"pull failed\" token=myco_agent_abcdefghijklmnop",
		"noise",
		"myco-check end ok",
	})
	if found["proxy"] != "http://***@127.0.0.1:7890" || found["service"] != "active" || found["end"] != "ok" {
		t.Fatalf("%v", found)
	}
	if len(hubs) != 1 || hubs[0].Direct != "200" || hubs[0].Proxy != "000" {
		t.Fatalf("%+v", hubs)
	}
	if len(log) != 1 || strings.Contains(log[0], "abcdefghijklmnop") {
		t.Fatalf("log not masked: %v", log)
	}
	if !sameHubURL("https://hub.example.com/w/team/", "https://hub.example.com") || sameHubURL("https://a.example", "https://b.example") {
		t.Fatal("sameHubURL")
	}
}
