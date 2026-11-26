package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

func TestScreenshotMasksUseClipOriginAndPixelScale(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var raw bytes.Buffer
	png.Encode(&raw, img)
	out, err := maskScreenshot(raw.Bytes(), []controlRect{{X: 30, Y: 40, Width: 20, Height: 10}}, 100, 200, &proto.PageViewport{X: 120, Y: 230, Width: 200, Height: 100})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if color.NRGBAModel.Convert(decoded.At(30, 30)).(color.NRGBA) != (color.NRGBA{A: 255}) {
		t.Fatal("scaled frame region not opaque black")
	}
	if color.NRGBAModel.Convert(decoded.At(80, 60)).(color.NRGBA) != (color.NRGBA{R: 255, A: 255}) {
		t.Fatal("mask shifted outside its frame")
	}
}

func TestScreenshotPrivacyIgnoresFragmentNavigation(t *testing.T) {
	root := proto.PageFrameID("root")
	before := screenshotPrivacyPlan{Documents: map[proto.PageFrameID]string{root: "https://example.com/t/123#reply11"}, Epochs: map[proto.PageFrameID]uint64{root: 7}}
	after := screenshotPrivacyPlan{Documents: map[proto.PageFrameID]string{root: "https://example.com/t/123#reply12"}, Epochs: map[proto.PageFrameID]uint64{root: 7}}
	if !sameScreenshotPrivacy(before, after) {
		t.Fatal("same-document fragment navigation rejected a safe screenshot")
	}
	after.Documents[root] = "https://example.com/t/124#reply12"
	if sameScreenshotPrivacy(before, after) {
		t.Fatal("different document accepted")
	}
	after.Documents[root] = before.Documents[root]
	after.Epochs[root]++
	if sameScreenshotPrivacy(before, after) {
		t.Fatal("replaced frame epoch accepted")
	}
}

func TestScreenshotPrivacyReasonSurvivesReplayError(t *testing.T) {
	for _, reason := range []screenshotPrivacyError{"protected_content", "privacy_unavailable", "frame_unavailable", "privacy_changed"} {
		failure := controlNativeError(reason)
		if failure.Code != "sensitive_snapshot" || failure.Reason != string(reason) {
			t.Fatalf("reason %s lost from component error: %+v", reason, failure)
		}
		result := replayErrorDTO(failure)
		if result.Code != "sensitive_snapshot" || result.Reason != string(reason) {
			t.Fatalf("reason %s lost from replay error: %+v", reason, result)
		}
	}
}

func TestNativeChromeUnavailableFramesAreMaskedBeforeArtifactStorage(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	var base string
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/sensitive" {
			fmt.Fprint(w, `<!doctype html><input type="password" value="demo-secret">`)
			return
		}
		if r.URL.Path == "/ad" {
			fmt.Fprint(w, `<!doctype html><body style="margin:0;background:rgb(230,30,10)"></body>`)
			return
		}
		fmt.Fprintf(w, `<!doctype html><body style="margin:0;height:2000px;background:rgb(10,40,200)"><iframe id="ad" style="border:0;position:absolute;left:120px;top:850px;width:200px;height:100px" src="%s/ad"></iframe></body>`, strings.Replace(base, "127.0.0.1", "ads.localhost", 1))
	}))
	defer fixture.Close()
	base = fixture.URL
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	call := func(method, path string, in any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw)).WithContext(ctx)
		r.Header.Set("X-Myco-Scope", "demo/mask")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	var profile profile
	json.Unmarshal(call("POST", "/profiles", map[string]string{"name": "Frame mask fixture"}).Body.Bytes(), &profile)
	call("POST", "/profiles/"+profile.ID+"/agents", map[string]bool{"allowed": true})
	var opened struct {
		Session string `json:"session"`
	}
	json.Unmarshal(call("POST", "/control/open", map[string]any{"request_id": "open", "profile": profile.ID, "url": base, "grant_id": "demo-grant", "actor": controlActor{ID: "demo-agent"}, "expires_at": time.Now().Add(15 * time.Minute)}).Body.Bytes(), &opened)
	e.mu.Lock()
	c := e.controls[opened.Session]
	e.mu.Unlock()
	p, _ := c.browser.page()
	p = p.Context(ctx)
	// Keep the historical child worlds across a real document navigation.
	// Destroyed iframe events can arrive after a capture has copied the map.
	c.browser.mu.Lock()
	historical := map[string]map[proto.RuntimeExecutionContextID]proto.PageFrameID{}
	root := c.browser.installs[p.TargetID].rootFrame
	topSID := c.browser.installs[p.TargetID].sid
	c.browser.contexts[string(topSID)][1000001] = root
	for sid, worlds := range c.browser.contexts {
		historical[sid] = map[proto.RuntimeExecutionContextID]proto.PageFrameID{}
		for id, frame := range worlds {
			if frame != root {
				historical[sid][id] = frame
			}
		}
	}
	c.browser.mu.Unlock()
	if err = c.browser.setControlPanelHidden(ctx, p); err != nil {
		t.Fatal("stale top context vetoed a healthy collector", err)
	}
	c.browser.mu.Lock()
	delete(c.browser.contexts[string(topSID)], 1000001)
	topWorlds := map[proto.RuntimeExecutionContextID]proto.PageFrameID{}
	for id, frame := range c.browser.contexts[string(topSID)] {
		if frame == root {
			topWorlds[id] = frame
			delete(c.browser.contexts[string(topSID)], id)
		}
	}
	c.browser.mu.Unlock()
	missingTopErr := c.browser.setControlPanelHidden(ctx, p)
	c.browser.mu.Lock()
	for id, frame := range topWorlds {
		c.browser.contexts[string(topSID)][id] = frame
	}
	c.browser.mu.Unlock()
	if missingTopErr == nil {
		t.Fatal("capture accepted no live top collector")
	}
	if err = p.Navigate(base + "/after-navigation"); err != nil {
		t.Fatal(err)
	}
	if err = c.waitReady(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err = (proto.EmulationSetDeviceMetricsOverride{Width: 800, Height: 600, DeviceScaleFactor: 2, Mobile: false}).Call(p); err != nil {
		t.Fatal(err)
	}
	if err = p.WaitLoad(); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(`()=>scrollTo(0,650)`); err != nil {
		t.Fatal(err)
	}
	if err = c.present(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Prove the inaccessible renderer really contains pixels to redact.
	raw, err := (proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng, FromSurface: true}).Call(p)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := png.Decode(bytes.NewReader(raw.Data))
	if err != nil {
		t.Fatal(err)
	}
	if sample := color.NRGBAModel.Convert(visible.At(440, 500)).(color.NRGBA); sample.R < 200 || sample.B > 30 {
		t.Fatal("ad fixture did not render", sample)
	}
	var tree proto.PageGetFrameTreeResult
	if err = c.call(ctx, c.browser.primarySession(p.TargetID), "Page.getFrameTree", nil, &tree); err != nil {
		t.Fatal(err)
	}
	ad, err := p.Element("#ad")
	if err != nil {
		t.Fatal(err)
	}
	node, err := ad.Describe(0, false)
	if err != nil {
		t.Fatal(err)
	}
	child := node.FrameID
	if child == "" {
		t.Fatal("ad frame owner has no content ID")
	}
	c.browser.mu.Lock()
	for _, worlds := range c.browser.contexts {
		for id, frame := range worlds {
			if frame == child {
				delete(worlds, id)
			}
		}
	}
	for sid, worlds := range historical {
		if c.browser.contexts[sid] == nil {
			c.browser.contexts[sid] = map[proto.RuntimeExecutionContextID]proto.PageFrameID{}
		}
		for id, frame := range worlds {
			if frame != child {
				c.browser.contexts[sid][id] = frame
			}
		}
	}
	// A removed OOPIF renderer must never veto top-frame panel hiding.
	c.browser.contexts["detached-ad-session"] = map[proto.RuntimeExecutionContextID]proto.PageFrameID{1000000: "detached-ad"}
	c.browser.frames["detached-ad"] = frameInfo{Parent: tree.FrameTree.Frame.ID, URL: "https://ads.example/old"}
	c.browser.mu.Unlock()
	capture := func(name string, fields map[string]any) {
		t.Helper()
		fields["request_id"] = name
		fields["session"] = opened.Session
		var result struct {
			Screenshot struct {
				ID string `json:"id"`
			} `json:"screenshot"`
		}
		json.Unmarshal(call("POST", "/control/screenshot", fields).Body.Bytes(), &result)
		img, err := png.Decode(call("GET", "/artifacts/"+result.Screenshot.ID, nil).Body)
		if err != nil {
			t.Fatal(err)
		}
		// Scan the entire stored artifact directly, avoiding per-pixel interface
		// conversion/allocation overhead under the race detector.
		pixels := image.NewNRGBA(img.Bounds())
		draw.Draw(pixels, pixels.Bounds(), img, img.Bounds().Min, draw.Src)
		black, public := 0, 0
		for y := pixels.Bounds().Min.Y; y < pixels.Bounds().Max.Y; y++ {
			for x := pixels.Bounds().Min.X; x < pixels.Bounds().Max.X; x++ {
				i := pixels.PixOffset(x, y)
				pixel := color.NRGBA{R: pixels.Pix[i], G: pixels.Pix[i+1], B: pixels.Pix[i+2], A: pixels.Pix[i+3]}
				if pixel.R > 200 && pixel.G < 60 && pixel.B < 40 {
					t.Fatalf("%s artifact leaked iframe pixels at %d,%d", name, x, y)
				}
				if pixel.R == 0 && pixel.G == 0 && pixel.B == 0 && pixel.A == 255 {
					black++
				}
				if pixel.B > 150 && pixel.R < 40 {
					public++
				}
			}
		}
		if black < 1000 || public < 1000 {
			t.Fatal("mask missing or hid readable page", name, black, public)
		}
	}
	capture("viewport", map[string]any{})
	capture("clip", map[string]any{"clip": &screenshotClip{X: 100, Y: 180, Width: 300, Height: 150}})
	capture("full", map[string]any{"full": true})
	if _, err = p.Eval(`()=>{document.querySelector('iframe').style.position='fixed';document.querySelector('iframe').style.top='200px'}`); err != nil {
		t.Fatal(err)
	}
	capture("full-fixed", map[string]any{"full": true})
	if _, err = p.Eval(`()=>{const input=document.createElement('input');input.type='password';input.value='demo-secret';document.body.append(input)}`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.captureImage(ctx, p, controlInput{}); err == nil || err.Error() != "sensitive_snapshot" {
		t.Fatal("readable sensitive field accepted", err)
	}
	if _, err = p.Eval(`()=>{document.querySelector('input').remove();const f=document.createElement('iframe');f.id='readable';f.src='/sensitive';document.body.append(f)}`); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var sensitiveFrame proto.PageFrameID
		c.browser.mu.Lock()
		for id, f := range c.browser.frames {
			if strings.HasSuffix(f.URL, "/sensitive") {
				sensitiveFrame = id
			}
		}
		c.browser.mu.Unlock()
		readable := false
		for _, f := range c.framesFor(p) {
			if f.id == sensitiveFrame {
				readable = true
			}
		}
		if readable {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("same-origin private frame not readable")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err = c.captureImage(ctx, p, controlInput{}); err == nil || err.Error() != "sensitive_snapshot" {
		t.Fatal("readable iframe password accepted", err)
	}

}
