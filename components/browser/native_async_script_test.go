package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func TestScriptEvalErrorIsSafeAndSpecific(t *testing.T) {
	private := "private-fixture-value"
	err := &rod.EvalError{RuntimeExceptionDetails: &proto.RuntimeExceptionDetails{
		Exception: &proto.RuntimeRemoteObject{Description: private},
	}}
	mapped := scriptEvalError(err)
	if publicError(mapped) != "script_error" || strings.Contains(mapped.Error(), private) {
		t.Fatal("script exception leaked or lost its error code")
	}
	if scriptEvalError(context.DeadlineExceeded) != context.DeadlineExceeded {
		t.Fatal("non-script error was remapped")
	}
}

func TestNativeChromeAsyncScriptNodes(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	if len(installedBrowsers()) == 0 {
		t.Fatal("system Chrome/Edge required")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/value" {
			_, _ = w.Write([]byte("fetched-value"))
			return
		}
		_, _ = w.Write([]byte(`<!doctype html><p id="item">Item</p>`))
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	request := httptest.NewRequest("POST", "/profiles", bytes.NewBufferString(`{"name":"Async script fixture"}`)).WithContext(ctx)
	request.Header.Set("X-Myco-Scope", "demo/async-script")
	response := httptest.NewRecorder()
	e.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatal("profile creation failed", response.Code, response.Body.String())
	}
	var profile profile
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	s, err := e.openSession(ctx, "demo/async-script", profile.ID, true)
	if err != nil {
		t.Fatal("browser startup failed", err)
	}
	p, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	p = p.Context(ctx)
	if err := p.Navigate(fixture.URL); err != nil {
		t.Fatal(err)
	}
	if err := p.WaitLoad(); err != nil {
		t.Fatal(err)
	}
	run := &playRun{session: s, scope: "demo/async-script"}
	check := func(n node, want any) {
		t.Helper()
		got, err := run.act(ctx, n)
		if err != nil || got != want {
			t.Fatalf("%s: got=%v want=%v err=%v", n.Type, got, want, err)
		}
	}
	check(node{Type: "js", Code: "return await (await fetch('/value')).text()"}, "fetched-value")
	check(node{Type: "extract", What: "js", Target: &target{Locators: []locator{{Kind: "css", Value: "#item"}}}, Code: "return this.textContent + ':' + await (await fetch('/value')).text()"}, "Item:fetched-value")
	check(node{Type: "js", Code: "return 7"}, float64(7))
	for _, code := range []string{"return await (", "throw Error('private-fixture-value')"} {
		_, err := run.act(ctx, node{Type: "js", Code: code})
		if err == nil || publicError(err) != "script_error" || strings.Contains(err.Error(), "private-fixture-value") {
			t.Fatal("script failure was not safely classified", err)
		}
	}
}
