package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

func TestRealPeerVideoAndControl(t *testing.T) {
	for _, control := range []bool{true, false} {
		t.Run(fmt.Sprintf("control=%t", control), func(t *testing.T) { testPeer(t, control) })
	}
}
func testPeer(t *testing.T, control bool) {
	input := make(chan inputEvent, 1)
	e := &engine{cfg: settings{Secret: strings.Repeat("s", 64), PairingCode: "abcdef12", Control: true}, peers: map[string]*peer{}, canControl: func() bool { return true }, input: func(ev inputEvent) error { input <- ev; return nil }, start: func(ctx context.Context, tr *webrtc.TrackLocalStaticRTP) error {
		tick := time.NewTicker(15 * time.Millisecond)
		defer tick.Stop()
		var seq uint16
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tick.C:
				seq++
				_ = tr.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: uint32(seq) * 3000, SSRC: 1, Marker: true}, Payload: []byte{0x65, 0x88, 0x84}})
			}
		}
	}}
	defer e.shutdown()
	server := httptest.NewServer(e.handler())
	defer server.Close()
	controller, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close()
	received := make(chan struct{}, 1)
	controller.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if _, _, er := tr.ReadRTP(); er == nil {
			received <- struct{}{}
		}
	})
	_, _ = controller.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly})
	dc, err := controller.CreateDataChannel("input", nil)
	if err != nil {
		t.Fatal(err)
	}
	dc.OnOpen(func() { _ = dc.SendText(`{"kind":"key","code":"KeyA","down":true}`) })
	off, _ := controller.CreateOffer(nil)
	gather := webrtc.GatheringCompletePromise(controller)
	_ = controller.SetLocalDescription(off)
	select {
	case <-gather:
	case <-time.After(8 * time.Second):
		t.Fatal("ICE gathering timed out")
	}
	body, _ := json.Marshal(offer{Description: description{Type: "offer", SDP: controller.LocalDescription().SDP}, PairingCode: "abcdef12", Control: control})
	req, _ := http.NewRequest("POST", server.URL+"/sessions/one", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.cfg.Secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}
	var answer description
	if err = json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if err = controller.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(10 * time.Second):
		t.Fatal("no video RTP arrived over real WebRTC")
	}
	if !control {
		select {
		case ev := <-input:
			t.Fatal("view-only session injected input", ev)
		case <-time.After(150 * time.Millisecond):
		}
		return
	}
	select {
	case ev := <-input:
		if ev.Code != "KeyA" || !ev.Down {
			t.Fatal(ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no keyboard input arrived")
	}
}
func TestPairingOriginRateLimitAndViewOnly(t *testing.T) {
	e := &engine{cfg: settings{Secret: strings.Repeat("s", 64), PairingCode: "abcdef12"}, peers: map[string]*peer{}, canControl: func() bool { return true }}
	call := func(code, origin string, control bool) int {
		b, _ := json.Marshal(offer{Description: description{Type: "offer", SDP: "v=0"}, PairingCode: code, Control: control})
		r := httptest.NewRequest("POST", "/sessions/one", bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+e.cfg.Secret)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, r)
		return w.Code
	}
	if got := call("abcdef12", "https://untrusted.example", false); got != 403 {
		t.Fatal(got)
	}
	if got := call("abcdef12", "", true); got != 403 {
		t.Fatal(got)
	}
	for i := 0; i < 5; i++ {
		if got := call("wrong", "", false); got != 403 {
			t.Fatal(got)
		}
	}
	if got := call("abcdef12", "", false); got != 429 {
		t.Fatal(got)
	}
}
func TestInputValidation(t *testing.T) {
	for _, ev := range []inputEvent{{Kind: "move", X: -1}, {Kind: "button", Button: 3}, {Kind: "key", Code: "run-shell"}, {Kind: "scroll", Delta: 1201}} {
		if ev.valid() {
			t.Fatal(ev)
		}
	}
	if !(inputEvent{Kind: "move", X: .5, Y: .5}).valid() {
		t.Fatal("valid pointer rejected")
	}
}

func TestCaptureDeviceNeverGuessesCameraOrAudio(t *testing.T) {
	output := "AVFoundation video devices:\n[0] Capture screen 0\n[1] FaceTime Camera\n[2] Capture screen 0\nAVFoundation audio devices:\n[9] Capture screen 0\n"
	if got := screenDevice(output); got != "2" {
		t.Fatal(got)
	}
	if got := screenDevice("AVFoundation video devices:\n[0] FaceTime Camera\nAVFoundation audio devices:\n[1] Capture screen 0"); got != "" {
		t.Fatal("selected audio/camera instead of a display", got)
	}
}

func TestShutdownWaitsForEncoderCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	e := &engine{peers: map[string]*peer{}}
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	e.peers["one"] = &peer{pc: pc, cancel: cancel}
	cleaned := false
	e.encoders.Add(1)
	go func() { defer e.encoders.Done(); <-ctx.Done(); cleaned = true }()
	e.shutdown()
	if !cleaned || !e.stopping || len(e.peers) != 0 {
		t.Fatal("shutdown returned before cleanup")
	}
}
