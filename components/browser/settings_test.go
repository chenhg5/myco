package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProfileSettingsValidation(t *testing.T) {
	for _, arg := range []string{"--start-maximized", "--disable-gpu", "--disable-background-timer-throttling", "--disable-backgrounding-occluded-windows", "--disable-renderer-backgrounding", "--window-size=1280,900", "--force-device-scale-factor=1.25", "--force-device-scale-factor=4", "--disable-features=A_b,C2", "--enable-features=A", "--blink-settings=imagesEnabled=false", "--blink-settings=imagesEnabled=true"} {
		if !launchArgOK(arg) {
			t.Errorf("valid flag rejected: %s", arg)
		}
	}
	for _, arg := range []string{"--no-sandbox", "--remote-debugging-port=1", "--user-data-dir=/tmp", "--load-extension=/tmp", "--disable-web-security", "--headless", "--enable-automation", "--force-webrtc-ip-handling-policy=default", "--proxy-server=x", "--host-resolver-rules=x", "--window-size=1,2", "--force-device-scale-factor=4.1", "--enable-features=A<file", "--disable-gpu=true", "--blink-settings=imagesEnabled=false,foo=true", "--disable-gpu\n--no-sandbox"} {
		if launchArgOK(arg) {
			t.Errorf("unsafe flag accepted: %q", arg)
		}
	}
	for _, server := range []string{"http://proxy.example:80", "https://proxy.example:443", "socks5://[::1]:1080"} {
		if _, err := proxyURL(server); err != nil {
			t.Errorf("valid proxy rejected: %s", server)
		}
	}
	for _, server := range []string{"http://u:p@host:80", "http://host", "ftp://host:80", "http://host:0", "http://host:65536", "http://host:80/path", "http://host:80?", "http://host:80#fragment", "http://host:80\r\nX:y"} {
		if _, err := proxyURL(server); err == nil {
			t.Errorf("invalid proxy accepted: %q", server)
		}
	}
	for _, in := range []profileSettingsInput{{UserAgent: "Bad\r\nHeader:x"}, {Lang: "en\nUS"}, {Lang: "--no-sandbox"}, {Args: []string{"--no-sandbox"}}, {Proxy: &proxySettingsInput{Server: "http://host:80", Bypass: []string{"x;*"}}}} {
		if _, _, err := validatedSettings(in, nil, ""); err == nil {
			t.Error("invalid settings accepted")
		}
	}
	settings, _, err := validatedSettings(profileSettingsInput{UserAgent: "Mozilla/5.0 Chrome/159.0.0.0", Lang: "zh-CN", Args: []string{"--window-size=1200,800"}, Proxy: &proxySettingsInput{Server: "http://host:80", Bypass: []string{"*.local", "<local>"}}}, nil, "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	flags := strings.Join(profileLaunchArgs(settings, "http://127.0.0.1:12345"), "\n")
	for _, want := range []string{"--force-webrtc-ip-handling-policy=disable_non_proxied_udp", "--proxy-server=http://127.0.0.1:12345", "--proxy-bypass-list=*.local;<local>", "--user-agent=Mozilla/5.0 Chrome/159.0.0.0", "--lang=zh-CN", "--accept-lang=zh-CN"} {
		if !strings.Contains(flags, want) {
			t.Errorf("missing flag %s", want)
		}
	}
	if strings.Contains(flags, "fixture-password") || strings.Contains(flags, "http://host:80") {
		t.Fatal("upstream credentials/address exposed to Chrome")
	}
}
func TestProfileSettingsPasswordLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	e, err := newEngine(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	call := func(method, path, body string) string {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Myco-Scope", "settings-fixture")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "private-fixture-password") {
			t.Fatal("password returned")
		}
		return w.Body.String()
	}
	var p profile
	json.Unmarshal([]byte(call("POST", "/profiles", `{"name":"Work"}`)), &p)
	patch := func(proxy string) profile {
		t.Helper()
		var out profile
		json.Unmarshal([]byte(call("PATCH", "/profiles/"+p.ID, `{"settings":{"proxy":`+proxy+`,"user_agent":"","lang":"","args":[]}}`)), &out)
		return out
	}
	p = patch(`{"server":"http://proxy.example:8080","bypass":[],"username":"u","password":"private-fixture-password"}`)
	if !p.Settings.Proxy.HasPassword {
		t.Fatal("password not stored")
	}
	call("GET", "/status", "")
	fi, err := os.Stat(filepath.Join(dir, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0600 {
		t.Fatal("profile index is not private")
	}
	reloaded, err := newEngine(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.passwords[p.ID] != "private-fixture-password" {
		t.Fatal("secret did not survive restart")
	}
	e = reloaded
	p = patch(`{"server":"http://proxy.example:8080","bypass":["<local>"],"username":"u"}`)
	if !p.Settings.Proxy.HasPassword || e.passwords[p.ID] != "private-fixture-password" {
		t.Fatal("omitted password was not retained")
	}
	p = patch(`{"server":"http://proxy.example:8080","bypass":[],"username":"u","password":""}`)
	if p.Settings.Proxy.HasPassword || e.passwords[p.ID] != "" {
		t.Fatal("empty password did not clear")
	}
	patch(`{"server":"http://proxy.example:8080","bypass":[],"username":"u","password":"private-fixture-password"}`)
	p = patch(`null`)
	if p.Settings.Proxy != nil || e.passwords[p.ID] != "" {
		t.Fatal("proxy:null retained credentials")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "profiles.json"))
	if strings.Contains(string(raw), "private-fixture-password") {
		t.Fatal("cleared secret remained in index")
	}
}

func TestProfilePasswordRetainedOnlyForSameProxyIdentity(t *testing.T) {
	previous := &proxySettings{Server: "http://proxy.example:8080", Username: "owner"}
	for _, tc := range []struct {
		name, server, user string
		keep               bool
	}{
		{"same", previous.Server, previous.Username, true},
		{"server", "http://other.example:8080", previous.Username, false},
		{"scheme", "https://proxy.example:8080", previous.Username, false},
		{"username", previous.Server, "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := profileSettingsInput{Proxy: &proxySettingsInput{Server: tc.server, Username: tc.user}}
			settings, password, err := validatedSettings(input, previous, "private-fixture-password")
			if err != nil {
				t.Fatal(err)
			}
			if (password != "") != tc.keep || settings.Proxy.HasPassword != tc.keep {
				t.Fatal("saved credential retained for the wrong identity")
			}
		})
	}
	if flags := strings.Join(profileLaunchArgs(profileSettings{}, ""), " "); strings.Contains(flags, "webrtc") {
		t.Fatal("unproxied profile changed WebRTC policy")
	}
}
