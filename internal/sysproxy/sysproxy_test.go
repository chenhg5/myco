package sysproxy

import (
	"errors"
	"testing"
	"time"
)

// fake replaces the system reading and the probe for one test.
func fake(t *testing.T, s Setting, up bool) *int {
	t.Helper()
	dials := 0
	oldRead, oldDial, oldNow := read, dial, nowFunc
	read = func() Setting { return s }
	dial = func(string, time.Duration) error {
		dials++
		if up {
			return nil
		}
		return errors.New("refused")
	}
	reset := func() {
		mu.Lock()
		cached, readAt, probes = Setting{}, time.Time{}, map[string]probe{}
		mu.Unlock()
	}
	reset()
	t.Cleanup(func() { read, dial, nowFunc = oldRead, oldDial, oldNow; reset() })
	return &dials
}

func TestParseWindows(t *testing.T) {
	cases := []struct {
		enable           uint64
		server, override string
		want             string
	}{
		{1, "127.0.0.1:7890", "", "http://127.0.0.1:7890"},
		{0, "127.0.0.1:7890", "", ""},
		{1, "", "", ""},
		{1, "http=10.0.0.1:8080;https=10.0.0.1:8443", "", "http://10.0.0.1:8443"},
		{1, "http=10.0.0.1:8080", "", "http://10.0.0.1:8080"},
		{1, "socks=127.0.0.1:1080", "", "socks5://127.0.0.1:1080"},
		{1, "ftp=1.2.3.4:21", "", ""},
		{1, "127.0.0.1:99999", "", ""},
	}
	for _, c := range cases {
		s := parseWindows(c.enable, c.server, c.override)
		got := ""
		if s.Proxy != nil {
			got = s.Proxy.String()
		}
		if got != c.want {
			t.Errorf("parseWindows(%d, %q) = %q, want %q", c.enable, c.server, got, c.want)
		}
	}
	s := parseWindows(1, "127.0.0.1:7890", "localhost;127.*;10.*;*.corp.example;<local>")
	if len(s.Bypass) != 5 {
		t.Fatalf("bypass: %v", s.Bypass)
	}
}

func TestParseScutil(t *testing.T) {
	mac := `<dictionary> {
  ExceptionsList : <array> {
    0 : *.local
    1 : 169.254/16
    2 : example.org
  }
  FTPPassive : 1
  HTTPEnable : 1
  HTTPPort : 10808
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 1
  HTTPSPort : 10809
  HTTPSProxy : 127.0.0.1
}
`
	s := parseScutil(mac)
	if s.Proxy == nil || s.Proxy.String() != "http://127.0.0.1:10809" {
		t.Fatalf("proxy: %v", s.Proxy)
	}
	if len(s.Bypass) != 3 || s.Bypass[1] != "169.254/16" {
		t.Fatalf("bypass: %v", s.Bypass)
	}
	if s := parseScutil("<dictionary> {\n  HTTPSEnable : 0\n}\n"); s.Proxy != nil {
		t.Fatalf("off: %v", s.Proxy)
	}
	socks := "<dictionary> {\n  SOCKSEnable : 1\n  SOCKSPort : 1080\n  SOCKSProxy : 127.0.0.1\n}\n"
	if s := parseScutil(socks); s.Proxy == nil || s.Proxy.String() != "socks5://127.0.0.1:1080" {
		t.Fatalf("socks: %v", s.Proxy)
	}
}

func TestBypassed(t *testing.T) {
	s := Setting{Bypass: []string{"<local>", "10.*", "*.corp.example", ".lan.example", "169.254/16", "Example.ORG"}}
	for host, want := range map[string]bool{
		"localhost":          true,
		"127.0.0.1":          true,
		"[::1]":              true,
		"intranet":           true,
		"10.1.2.3":           true,
		"a.corp.example":     true,
		"corp.example":       true,
		"x.lan.example":      true,
		"lan.example":        true,
		"169.254.10.1":       true,
		"example.org":        true,
		"github.com":         false,
		"api.github.com":     false,
		"11.1.2.3":           false,
		"notcorp.example":    false,
		"192.168.1.1":        false,
		"release-assets.com": false,
	} {
		if got := s.Bypassed(host); got != want {
			t.Errorf("Bypassed(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestForUsesListeningProxyOnly(t *testing.T) {
	s := parseWindows(1, "127.0.0.1:7890", "<local>")
	dials := fake(t, s, true)
	if u := For("github.com"); u == nil || u.Host != "127.0.0.1:7890" {
		t.Fatalf("For(github.com) = %v", u)
	}
	if u := For("intranet"); u != nil {
		t.Fatalf("bypassed host got %v", u)
	}
	For("api.github.com")
	if *dials != 1 {
		t.Fatalf("probe not cached: %d dials", *dials)
	}

	// A proxy app that quit and left the setting on: go direct.
	fake(t, s, false)
	if u := For("github.com"); u != nil {
		t.Fatalf("dead proxy used: %v", u)
	}

	fake(t, Setting{}, true)
	if u := For("github.com"); u != nil {
		t.Fatalf("no setting, got %v", u)
	}
}

func TestCurrentRereadsAfterTTL(t *testing.T) {
	n := 0
	fake(t, Setting{}, true)
	read = func() Setting { n++; return Setting{} }
	now := time.Unix(1000, 0)
	nowFunc = func() time.Time { return now }
	Current()
	Current()
	if n != 1 {
		t.Fatalf("read %d times, want 1", n)
	}
	now = now.Add(settingTTL + time.Second)
	Current()
	if n != 2 {
		t.Fatalf("read %d times after TTL, want 2", n)
	}
}
