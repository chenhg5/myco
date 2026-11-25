package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func TestNativeChromeClipUsesViewportPixelsAndNeverFullPageEvidence(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><style>body{margin:0;height:3000px;background:#004488}#sample{position:fixed;left:80px;top:120px;width:160px;height:90px;background:rgb(230,30,40)}</style><div id="sample"></div>`)
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	scope := "native/clip"
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(raw)).WithContext(ctx)
		request.Header.Set("X-Myco-Scope", scope)
		result := httptest.NewRecorder()
		e.handler().ServeHTTP(result, request)
		if result.Code != 200 {
			t.Fatalf("%s: %d %s", path, result.Code, result.Body.String())
		}
		return result
	}
	var p profile
	if err := json.Unmarshal(call("POST", "/profiles", map[string]string{"name": "Clip fixture"}).Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	call("POST", "/sessions", map[string]any{"profile": p.ID, "headless": true})
	session, err := e.session(scope, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := session.page()
	if err != nil {
		t.Fatal(err)
	}
	page = page.Context(ctx)
	if err := (proto.EmulationSetDeviceMetricsOverride{Width: 800, Height: 600, DeviceScaleFactor: 2, Mobile: false}).Call(page); err != nil {
		t.Fatal(err)
	}
	if err = page.Navigate(fixture.URL); err != nil {
		t.Fatal(err)
	}
	if err = page.WaitLoad(); err != nil {
		t.Fatal(err)
	}
	if _, err = page.Eval(`()=>scrollTo(0,900)`); err != nil {
		t.Fatal(err)
	}
	if err = page.Wait(rod.Eval(`()=>scrollY===900`)); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Features []string `json:"features"`
	}
	json.Unmarshal(call("GET", "/status", nil).Body.Bytes(), &status)
	if !slices.Contains(status.Features, "screenshot_clip") || !slices.Contains(status.Features, "agent_control") {
		t.Fatal("clip not negotiated", status)
	}
	run := func(secret bool) runUpdate {
		t.Helper()
		var script script
		script.Graph.Nodes = []node{{ID: "crop", Type: "screenshot", Clip: &screenshotClip{X: 80, Y: 120, Width: 160, Height: 90}, Into: "image"}}
		options := runOptions{Profile: p.ID, Headless: true, StartAt: "crop", Script: script}
		if secret {
			options.Script.Params = []param{{Name: "token", Type: "secret", Required: true}}
			options.Secrets = map[string]string{"token": "fixture-secret"}
		}
		var started struct {
			ID string `json:"id"`
		}
		json.Unmarshal(call("POST", "/runs", options).Body.Bytes(), &started)
		for {
			var result runUpdate
			if err := json.Unmarshal(call("GET", "/runs/"+started.ID+"?after=0", nil).Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if finishedRun(result.State) {
				for {
					session.mu.Lock()
					free := session.controller == ""
					session.mu.Unlock()
					if free {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(10 * time.Millisecond):
					}
				}
				return result
			}
		}
	}
	result := run(false)
	if result.State != "succeeded" {
		t.Fatalf("crop failed: %+v", result)
	}
	id, ok := result.Output["image"].(string)
	if !ok || id == "" {
		t.Fatal("crop artifact missing", result)
	}
	for _, step := range result.Steps {
		if step.State == "ok" && step.Shot != id {
			t.Fatal("crop substituted full-page evidence", step)
		}
	}
	image, err := png.Decode(bytes.NewReader(call("GET", "/artifacts/"+id, nil).Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 320 || image.Bounds().Dy() != 180 {
		t.Fatal("CSS clip did not retain DPR2", image.Bounds())
	}
	red, green, blue, _ := image.At(160, 90).RGBA()
	if red < 50000 || green > 15000 || blue > 15000 {
		t.Fatalf("clip used document rather than viewport coordinates: %d %d %d", red, green, blue)
	}
	e.mu.Lock()
	before := len(e.artifacts)
	e.mu.Unlock()
	for _, secret := range []bool{true, false} {
		if !secret {
			if _, err = page.Eval(`()=>{const input=document.createElement('input');input.type='password';document.body.append(input)}`); err != nil {
				t.Fatal(err)
			}
		}
		blocked := run(secret)
		if blocked.State != "failed" || blocked.Error != "sensitive_snapshot" {
			t.Fatalf("secret/sensitive crop leaked: %+v", blocked)
		}
		for _, step := range blocked.Steps {
			if step.Shot != "" {
				t.Fatal("failed crop fell back to full-page evidence", step)
			}
		}
	}
	e.mu.Lock()
	after := len(e.artifacts)
	e.mu.Unlock()
	if after != before {
		t.Fatalf("sensitive crop persisted artifacts: %d -> %d", before, after)
	}
}
