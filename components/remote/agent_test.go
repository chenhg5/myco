package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func agentEngine(t *testing.T, control bool, input func(inputEvent) error) (*engine, func(path, body string) (int, map[string]any)) {
	t.Helper()
	e := &engine{cfg: settings{Secret: strings.Repeat("s", 64), PairingCode: "abcdef12", Control: control}, peers: map[string]*peer{},
		canControl: func() bool { return true }, input: input, geometry: func() [4]int { return [4]int{0, 0, 1512, 982} },
		snap: func(ctx context.Context, maxWidth int, format string) ([]byte, error) {
			var b bytes.Buffer
			_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 64, 40)), nil)
			return b.Bytes(), nil
		}}
	srv := httptest.NewServer(e.handler())
	t.Cleanup(srv.Close)
	call := func(path, body string) (int, map[string]any) {
		req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+e.cfg.Secret)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	return e, call
}

func TestAgentObserveThenInput(t *testing.T) {
	var mu sync.Mutex
	var got []inputEvent
	_, call := agentEngine(t, true, func(ev inputEvent) error { mu.Lock(); got = append(got, ev); mu.Unlock(); return nil })
	if code, out := call("/agent/observe", `{"format":"gif"}`); code != 400 || out["error"] != "invalid_argument" {
		t.Fatalf("bad format: %d %v", code, out)
	}
	code, obs := call("/agent/observe", `{}`)
	if code != 200 || obs["width"].(float64) != 64 || obs["height"].(float64) != 40 || obs["media_type"] != "image/jpeg" || obs["display_id"] != "main" || obs["image"] == "" {
		t.Fatalf("observe: %d %v", code, obs)
	}
	frame := obs["frame_id"].(string)
	// Nothing happens unless every event is valid, or for a frame never observed.
	if code, out := call("/agent/input", `{"frame_id":"`+frame+`","events":[{"kind":"move","x":0.5,"y":0.5},{"kind":"move","x":2,"y":0}]}`); code != 400 || out["error"] != "invalid_argument" {
		t.Fatalf("invalid batch: %d %v", code, out)
	}
	if code, out := call("/agent/input", `{"frame_id":"frm_other","events":[{"kind":"move","x":0.5,"y":0.5}]}`); code != 409 || out["error"] != "stale_frame" {
		t.Fatalf("unknown frame: %d %v", code, out)
	}
	if code, _ := call("/agent/input", `{"frame_id":"`+frame+`","events":[]}`); code != 400 {
		t.Fatalf("empty batch: %d", code)
	}
	if len(got) != 0 {
		t.Fatalf("rejected batches applied events: %v", got)
	}
	code, out := call("/agent/input", `{"frame_id":"`+frame+`","events":[{"kind":"button","x":0.25,"y":0.5,"button":0,"clicks":1,"down":true},{"kind":"button","x":0.25,"y":0.5,"button":0,"clicks":1,"down":false},{"kind":"text","text":"你好 myco"}]}`)
	if code != 200 || out["applied"].(float64) != 3 || len(got) != 3 || got[2].Text != "你好 myco" {
		t.Fatalf("input: %d %v %v", code, out, got)
	}
}

// A frame from before the display changed (resolution, arrangement) is stale.
func TestAgentInputRefusesAFrameOfAnotherGeometry(t *testing.T) {
	e, call := agentEngine(t, true, func(inputEvent) error { t.Fatal("applied input for a changed display"); return nil })
	_, obs := call("/agent/observe", `{}`)
	e.mu.Lock()
	e.geometry = func() [4]int { return [4]int{0, 0, 1920, 1080} }
	e.mu.Unlock()
	if code, out := call("/agent/input", `{"frame_id":"`+obs["frame_id"].(string)+`","events":[{"kind":"move","x":0,"y":0}]}`); code != 409 || out["error"] != "stale_frame" {
		t.Fatalf("changed geometry: %d %v", code, out)
	}
}

func TestAgentInputViewOnlyAndFailure(t *testing.T) {
	_, view := agentEngine(t, false, func(inputEvent) error { t.Fatal("view only applied input"); return nil })
	_, obs := view("/agent/observe", `{}`)
	if code, out := view("/agent/input", `{"frame_id":"`+obs["frame_id"].(string)+`","events":[{"kind":"move","x":0,"y":0}]}`); code != 403 || out["error"] != "control_not_allowed" {
		t.Fatalf("view only: %d %v", code, out)
	}
	n := 0
	_, call := agentEngine(t, true, func(inputEvent) error {
		n++
		if n == 3 {
			return errors.New("refused")
		}
		return nil
	})
	_, obs = call("/agent/observe", `{}`)
	code, out := call("/agent/input", `{"frame_id":"`+obs["frame_id"].(string)+`","events":[{"kind":"move","x":0,"y":0},{"kind":"move","x":0.1,"y":0},{"kind":"move","x":0.2,"y":0},{"kind":"move","x":0.3,"y":0}]}`)
	if code != 500 || out["error"] != "input_failed" || out["applied"].(float64) != 2 || n != 3 {
		t.Fatalf("failure: %d %v (%d calls)", code, out, n)
	}
}

// The App cancels a running batch (its grant was revoked) by dropping the
// request: the rest of the batch never happens.
func TestAgentInputStopsWhenTheRequestEnds(t *testing.T) {
	started := make(chan struct{}, 64)
	var mu sync.Mutex
	n := 0
	e, call := agentEngine(t, true, func(inputEvent) error { mu.Lock(); n++; mu.Unlock(); started <- struct{}{}; return nil })
	_, obs := call("/agent/observe", `{}`)
	events := make([]string, 64)
	for i := range events {
		events[i] = `{"kind":"move","x":0.5,"y":0.5}`
	}
	srv := httptest.NewServer(e.handler())
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/agent/input", strings.NewReader(`{"frame_id":"`+obs["frame_id"].(string)+`","events":[`+strings.Join(events, ",")+`]}`))
	req.Header.Set("Authorization", "Bearer "+e.cfg.Secret)
	go func() { <-started; <-started; cancel() }()
	_, _ = http.DefaultClient.Do(req)
	time.Sleep(200 * time.Millisecond)
	e.inputMu.Lock() // the batch has returned
	e.inputMu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	if n < 2 || n > 6 {
		t.Fatalf("applied %d of 64 after the request ended", n)
	}
}

func TestStatusAdvertisesAgentActions(t *testing.T) {
	e, _ := agentEngine(t, true, nil)
	srv := httptest.NewServer(e.handler())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/status", nil)
	req.Header.Set("Authorization", "Bearer "+e.cfg.Secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var st struct {
		Version string   `json:"version"`
		Agent   []string `json:"agent_actions"`
	}
	_ = json.NewDecoder(res.Body).Decode(&st)
	if st.Version != version || len(st.Agent) != 2 || st.Agent[1] != "remote.desktop.input" {
		t.Fatalf("status: %+v", st)
	}
}

func TestSnapshotArgsAndBMP(t *testing.T) {
	a := strings.Join(snapshotArgs(1280), " ")
	if !strings.Contains(a, `select=gte(n\,3),scale='min(1280,iw)':-2`) || !strings.Contains(a, "-frames:v 1") || !strings.Contains(a, "-c:v bmp -pix_fmt bgr24 -f image2pipe pipe:1") {
		t.Fatalf("args: %s", a)
	}
	// A 2x2 bottom-up BMP: red, green / blue, white (rows padded to 4 bytes).
	px := []byte{0xff, 0, 0, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0xff, 0, 0xff, 0, 0, 0}
	head := make([]byte, 54)
	copy(head, "BM")
	binary.LittleEndian.PutUint32(head[10:], 54)
	binary.LittleEndian.PutUint32(head[14:], 40)
	binary.LittleEndian.PutUint32(head[18:], 2)
	binary.LittleEndian.PutUint32(head[22:], 2)
	binary.LittleEndian.PutUint16(head[26:], 1)
	binary.LittleEndian.PutUint16(head[28:], 24)
	img, err := decodeBMP(append(head, px...))
	if err != nil {
		t.Fatal(err)
	}
	want := map[[2]int][3]uint8{{0, 0}: {0xff, 0, 0}, {1, 0}: {0, 0xff, 0}, {0, 1}: {0, 0, 0xff}, {1, 1}: {0xff, 0xff, 0xff}}
	for p, c := range want {
		got := img.RGBAAt(p[0], p[1])
		if got.R != c[0] || got.G != c[1] || got.B != c[2] {
			t.Fatalf("pixel %v: %v want %v", p, got, c)
		}
	}
	if _, err := decodeBMP(head[:20]); err == nil {
		t.Fatal("a short file decoded")
	}
}
