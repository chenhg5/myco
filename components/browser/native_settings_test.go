package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

func TestNativeChromeProfileSettingsAndBypass(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with an installed Chrome/Edge")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body>direct <span id="lang">%s</span></body></html>`, r.Header.Get("Accept-Language"))
	}))
	defer direct.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fixture" {
			if u, p, ok := r.BasicAuth(); ok || u != "" || p != "" {
				t.Error("origin Authorization was fabricated")
			}
			if r.Header.Get("Proxy-Authorization") == "" {
				t.Error("upstream proxy authentication missing")
			}
			fmt.Fprintf(w, `<html><body>proxied <span id="lang">%s</span></body></html>`, r.Header.Get("Accept-Language"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	invoke := func(method, path string, in any, out any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw))).WithContext(ctx)
		r.Header.Set("X-Myco-Scope", "native-settings")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var profile profile
	invoke("POST", "/profiles", map[string]string{"name": "Proxy fixture"}, &profile)
	password := "fixture-password"
	settings := profileSettingsInput{UserAgent: "MYCO-settings-fixture/1.0", Lang: "zh-CN", Args: []string{"--window-size=1280,900"}, Proxy: &proxySettingsInput{Server: upstream.URL, Username: "fixture-user", Password: &password, Bypass: []string{"<-loopback>", "localhost"}}}
	invoke("PATCH", "/profiles/"+profile.ID, map[string]any{"settings": settings}, nil)
	s, err := e.openSession(ctx, "native-settings", profile.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	foundPolicy := false
	actualArgs, argsErr := browserCommandArguments(s.cmd)
	if argsErr != nil {
		t.Fatal(argsErr)
	}
	for _, arg := range actualArgs {
		foundPolicy = foundPolicy || arg == "--force-webrtc-ip-handling-policy=disable_non_proxied_udp"
	}
	if !foundPolicy {
		t.Fatal("proxied Chrome lacks WebRTC UDP protection")
	}
	page, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	page = page.Context(ctx)
	if err = page.Navigate(direct.URL + "/fixture"); err != nil {
		t.Fatal(err)
	}
	if err = page.Wait(rod.Eval(`()=>document.body?.innerText.includes('proxied')`)); err != nil {
		t.Fatal("proxy was bypassed", err)
	}
	value, err := page.Eval(`()=>({ua:navigator.userAgent,language:navigator.language,header:document.querySelector('#lang').textContent})`)
	if err != nil {
		t.Fatal(err)
	}
	if value.Value.Get("ua").Str() != settings.UserAgent || value.Value.Get("language").Str() != "zh-CN" || !strings.Contains(value.Value.Get("header").Str(), "zh-CN") {
		t.Fatal("UA/language launch settings did not reach the page:", value.Value)
	}
	bypassURL := strings.Replace(direct.URL, "127.0.0.1", "localhost", 1) + "/fixture"
	if err = page.Navigate(bypassURL); err != nil {
		t.Fatal(err)
	}
	if err = page.Wait(rod.Eval(`()=>document.body?.innerText.includes('direct')`)); err != nil {
		t.Fatal("explicit bypass did not reach direct server", err)
	}
	settings = profileSettingsInput{UserAgent: "MYCO-settings-fixture/2.0", Lang: "en-US", Args: []string{}}
	invoke("PATCH", "/profiles/"+profile.ID, map[string]any{"settings": settings}, nil)
	same, err := e.openSession(ctx, "native-settings", profile.ID, true)
	if err != nil || same != s {
		t.Fatal("saving settings replaced the current session", err)
	}
	value, err = page.Eval(`()=>navigator.userAgent`)
	if err != nil || value.Value.Str() != "MYCO-settings-fixture/1.0" {
		t.Fatal("running session changed settings", err)
	}
	localProxy := s.proxy.Address()
	s.close()
	s, err = e.openSession(ctx, "native-settings", profile.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if s.proxy != nil {
		t.Fatal("cleared proxy restarted a forwarder")
	}
	page, err = s.page()
	if err != nil {
		t.Fatal(err)
	}
	value, err = page.Context(ctx).Eval(`()=>navigator.userAgent`)
	if err != nil || value.Value.Str() != settings.UserAgent {
		t.Fatal("new session did not use saved UA", err)
	}
	page = page.Context(ctx)
	if err = page.Navigate(direct.URL + "/fixture"); err != nil {
		t.Fatal(err)
	}
	if err = page.Wait(rod.Eval(`()=>document.body?.innerText.includes('direct')`)); err != nil {
		t.Fatal("cleared proxy did not restore direct traffic", err)
	}
	value, err = page.Eval(`()=>({language:navigator.language,header:document.querySelector('#lang').textContent})`)
	if err != nil || value.Value.Get("language").Str() != "en-US" || !strings.Contains(value.Value.Get("header").Str(), "en-US") {
		t.Fatal("new session did not apply saved language", err)
	}
	actualArgs, argsErr = browserCommandArguments(s.cmd)
	if argsErr != nil {
		t.Fatal(argsErr)
	}
	for _, arg := range actualArgs {
		if strings.Contains(arg, password) || strings.Contains(arg, localProxy) {
			t.Fatal("old proxy settings leaked into new process")
		}
	}
}
