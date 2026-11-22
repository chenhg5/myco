// myco-remote is an optional process. The desktop App starts it with a private
// stdin configuration and stops it by closing stdin. It never reads Hub tokens.
package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

const protocol = 1
const version = "0.3.0"

type description struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}
type offer struct {
	Description description `json:"description"`
	PairingCode string      `json:"pairing_code"`
	Control     bool        `json:"control"`
}
type settings struct {
	Secret      string             `json:"secret"`
	PairingCode string             `json:"pairing_code"`
	Control     bool               `json:"control"`
	ICEServers  []webrtc.ICEServer `json:"ice_servers,omitempty"`
}
type peer struct {
	pc     *webrtc.PeerConnection
	cancel context.CancelFunc
	err    string
	at     time.Time
}
type engine struct {
	stopping   bool
	encoders   sync.WaitGroup
	lastErrors map[string]string
	cfg        settings
	mu         sync.Mutex
	inputMu    sync.Mutex
	peers      map[string]*peer
	failures   int
	retry      time.Time
	start      func(context.Context, *webrtc.TrackLocalStaticRTP) error
	input      func(inputEvent) error
	canControl func() bool
	// Agent actions (agent.go).
	snap     func(ctx context.Context, maxWidth int, format string) ([]byte, error)
	snapMu   sync.Mutex
	frames   []frame
	geometry func() [4]int
}

func (e *engine) controlAllowed() bool {
	if e.canControl != nil {
		return e.canControl()
	}
	return inputSupported()
}
func (e *engine) close(id string) {
	e.mu.Lock()
	p := e.peers[id]
	if p != nil && p.err != "" {
		if e.lastErrors == nil {
			e.lastErrors = map[string]string{}
		}
		if len(e.lastErrors) > 64 {
			for key := range e.lastErrors {
				delete(e.lastErrors, key)
				break
			}
		}
		e.lastErrors[id] = p.err
	}
	if p != nil {
		e.inputMu.Lock()
		p.cancel()
		releaseKeys()
		e.inputMu.Unlock()
	}
	delete(e.peers, id)
	e.mu.Unlock()
	if p != nil {
		p.cancel()
		_ = p.pc.Close()
	}
}
func (e *engine) handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		defer e.mu.Unlock()
		states := map[string]string{}
		for id, message := range e.lastErrors {
			states[id] = message
		}
		for id, p := range e.peers {
			if p != nil {
				states[id] = p.err
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"protocol": protocol, "version": version, "sessions": states, "control": e.controlAllowed(), "input_features": []string{"text"}, "agent_actions": agentActions})
	})
	m.HandleFunc("POST /sessions/{id}", e.connect)
	m.HandleFunc("POST /agent/observe", e.observe)
	m.HandleFunc("POST /agent/input", e.agentInput)
	m.HandleFunc("DELETE /sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		e.close(r.PathValue("id"))
		e.mu.Lock()
		delete(e.lastErrors, r.PathValue("id"))
		e.mu.Unlock()
		w.WriteHeader(204)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		// Only the parent talks to this loopback port. Web pages cannot open it.
		if r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(e.cfg.Secret)) != 1 {
			http.Error(w, `{"error":"permission_denied"}`, 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 70<<10)
		m.ServeHTTP(w, r)
	})
}
func (e *engine) connect(w http.ResponseWriter, r *http.Request) {
	var in offer
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Description.Type != "offer" || !strings.HasPrefix(in.Description.SDP, "v=0") {
		http.Error(w, `{"error":"invalid_offer"}`, 400)
		return
	}
	e.mu.Lock()
	if e.stopping {
		e.mu.Unlock()
		http.Error(w, `{"error":"component_stopping"}`, 503)
		return
	}
	if time.Now().Before(e.retry) {
		e.mu.Unlock()
		http.Error(w, `{"error":"pairing_paused"}`, 429)
		return
	}
	if subtle.ConstantTimeCompare([]byte(in.PairingCode), []byte(e.cfg.PairingCode)) != 1 {
		e.failures++
		if e.failures >= 5 {
			e.retry = time.Now().Add(time.Minute)
			e.failures = 0
		}
		e.mu.Unlock()
		http.Error(w, `{"error":"wrong_pairing_code"}`, 403)
		return
	}
	if in.Control && (!e.cfg.Control || !e.controlAllowed()) {
		e.mu.Unlock()
		http.Error(w, `{"error":"control_not_allowed"}`, 403)
		return
	}
	if len(e.peers) > 0 {
		e.mu.Unlock()
		http.Error(w, `{"error":"computer_busy"}`, 409)
		return
	}
	// Reserve before allocating a peer so concurrent requests cannot start capture twice.
	id := r.PathValue("id")
	e.peers[id] = nil
	e.mu.Unlock()
	success := false
	defer func() {
		if !success {
			e.close(id)
		}
	}()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: e.cfg.ICEServers})
	if err != nil {
		http.Error(w, `{"error":"connection_failed"}`, 500)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &peer{pc: pc, cancel: cancel, at: time.Now()}
	e.mu.Lock()
	e.peers[id] = p
	e.mu.Unlock()
	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"}, "screen", "myco")
	if err != nil {
		http.Error(w, `{"error":"video_failed"}`, 500)
		return
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		http.Error(w, `{"error":"video_failed"}`, 500)
		return
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, er := sender.Read(buf); er != nil {
				return
			}
		}
	}()
	var once sync.Once
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			once.Do(func() {
				e.mu.Lock()
				if e.stopping {
					e.mu.Unlock()
					return
				}
				e.encoders.Add(1)
				e.mu.Unlock()
				go func() {
					defer e.encoders.Done()
					if er := e.start(ctx, track); er != nil && ctx.Err() == nil {
						e.mu.Lock()
						p.err = "screen_capture_failed"
						e.mu.Unlock()
						cancel()
						_ = pc.Close()
					}
				}()
			})
		}
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			cancel()
			go e.close(id)
		}
	})
	var inputAccepted bool
	var channelMu sync.Mutex
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		channelMu.Lock()
		defer channelMu.Unlock()
		if dc.Label() != "input" || !in.Control || inputAccepted {
			_ = dc.Close()
			return
		}
		inputAccepted = true
		dc.OnOpen(func() { _ = dc.SendText(`{"type":"capabilities","input_features":["text"]}`) })
		var count int
		var window time.Time
		var messageMu sync.Mutex
		var inputClosed bool
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			messageMu.Lock()
			defer messageMu.Unlock()
			if len(msg.Data) > 2048 {
				return
			}
			if time.Since(window) > time.Second {
				count = 0
				window = time.Now()
			}
			count++
			var ev inputEvent
			if json.Unmarshal(msg.Data, &ev) != nil || !ev.valid() {
				return
			}
			// Never drop releases: excess pointer traffic must not leave a key held.
			if count > 300 && ev.Kind != "release" && !((ev.Kind == "key" || ev.Kind == "button") && !ev.Down) {
				return
			}
			{
				e.inputMu.Lock()
				if ctx.Err() == nil && !inputClosed {
					_ = e.input(ev)
				}
				e.inputMu.Unlock()
			}
		})
		dc.OnClose(func() { e.inputMu.Lock(); inputClosed = true; releaseKeys(); e.inputMu.Unlock() })
	})
	if pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: in.Description.SDP}) != nil {
		http.Error(w, `{"error":"invalid_offer"}`, 400)
		return
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		http.Error(w, `{"error":"connection_failed"}`, 500)
		return
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if pc.SetLocalDescription(answer) != nil {
		http.Error(w, `{"error":"connection_failed"}`, 500)
		return
	}
	select {
	case <-gathered:
	case <-r.Context().Done():
		return
	case <-time.After(12 * time.Second):
		http.Error(w, `{"error":"connection_timeout"}`, 504)
		return
	}
	success = true
	json.NewEncoder(w).Encode(description{Type: "answer", SDP: pc.LocalDescription().SDP})
	// Offers which never connect must not reserve the encoder forever.
	go func() {
		select {
		case <-time.After(25 * time.Second):
			if pc.ConnectionState() != webrtc.PeerConnectionStateConnected {
				e.close(id)
			}
		case <-ctx.Done():
		}
	}()
}
func main() {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadBytes('\n')
	if err != nil || len(line) > 64<<10 {
		os.Exit(2)
	}
	var cfg settings
	if json.Unmarshal(line, &cfg) != nil || len(cfg.Secret) < 32 || len(cfg.PairingCode) < 6 {
		os.Exit(2)
	}
	if cfg.Control {
		requestInputPermission()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	e := &engine{cfg: cfg, peers: map[string]*peer{}, start: capture, input: inject}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	// Stdout is a private pipe to the parent, not a terminal or a log file.
	fmt.Printf("{\"address\":%q,\"protocol\":1}\n", ln.Addr().String())
	srv := &http.Server{Handler: e.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, MaxHeaderBytes: 8192}
	go func() { _, _ = io.Copy(io.Discard, reader); cancel() }()
	shutdownDone := make(chan struct{})
	go func() { <-ctx.Done(); e.shutdown(); _ = srv.Close(); close(shutdownDone) }()

	_ = srv.Serve(ln)
	cancel()
	<-shutdownDone
}

// Wait for encoders to be reaped before exiting on parent EOF. In particular,
// a Mac App crash must not leave a detached capture process behind.
func (e *engine) shutdown() {
	e.mu.Lock()
	e.stopping = true
	ids := []string{}
	for id := range e.peers {
		ids = append(ids, id)
	}
	e.mu.Unlock()
	for _, id := range ids {
		e.close(id)
	}
	e.encoders.Wait()
}
