package main

// Agent actions (remote.desktop.observe and remote.desktop.input). The App
// calls these only for a job its Hub checked against a grant the computer's
// owner approved, and checks the grant again first. A still frame goes back
// to the App alone; input names the frame it was decided on, and every key
// or button it pressed is released when the batch ends or fails.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// agentActions are what /status advertises (the App registers only these).
var agentActions = []string{"remote.desktop.observe", "remote.desktop.input"}

const (
	frameTTL      = 2 * time.Minute // input must refer to a frame this recent
	framesKept    = 16
	snapshotLimit = 8 << 20
	inputGap      = 15 * time.Millisecond // lets apps take each event in turn
)

type frame struct {
	id string
	at time.Time
}
type observeRequest struct {
	MaxWidth int    `json:"max_width"`
	Format   string `json:"format"`
}
type observation struct {
	FrameID    string    `json:"frame_id"`
	CapturedAt time.Time `json:"captured_at"`
	DisplayID  string    `json:"display_id"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	MediaType  string    `json:"media_type"`
	Image      []byte    `json:"image"`
}
type inputRequest struct {
	FrameID string       `json:"frame_id"`
	Events  []inputEvent `json:"events"`
}

func agentError(w http.ResponseWriter, status int, code string, applied *int) {
	body := map[string]any{"error": code}
	if applied != nil {
		body["applied"] = *applied
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (e *engine) observe(w http.ResponseWriter, r *http.Request) {
	var in observeRequest
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		agentError(w, 400, "invalid_argument", nil)
		return
	}
	if in.Format == "" {
		in.Format = "jpeg"
	}
	if in.MaxWidth == 0 {
		in.MaxWidth = 1280
	}
	if (in.Format != "jpeg" && in.Format != "png") || in.MaxWidth < 320 || in.MaxWidth > 2560 {
		agentError(w, 400, "invalid_argument", nil)
		return
	}
	// One capture at a time: each one starts an encoder.
	e.snapMu.Lock()
	defer e.snapMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	snap := e.snap
	if snap == nil {
		snap = snapshot
	}
	data, err := snap(ctx, in.MaxWidth, in.Format)
	if err != nil {
		code := "screen_capture_failed"
		if m := err.Error(); m == "encoder_missing" || m == "no_screen" || m == "unsupported_platform" {
			code = m
		}
		agentError(w, 500, code, nil)
		return
	}
	cfg, kind, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || kind != in.Format {
		agentError(w, 500, "screen_capture_failed", nil)
		return
	}
	var id [12]byte
	_, _ = rand.Read(id[:])
	f := frame{id: "frm_" + hex.EncodeToString(id[:]), at: time.Now()}
	e.mu.Lock()
	e.frames = append(e.frames, f)
	if len(e.frames) > framesKept {
		e.frames = e.frames[len(e.frames)-framesKept:]
	}
	e.mu.Unlock()
	_ = json.NewEncoder(w).Encode(observation{FrameID: f.id, CapturedAt: f.at.UTC(), DisplayID: "main", Width: cfg.Width, Height: cfg.Height,
		MediaType: "image/" + in.Format, Image: data})
}

// knownFrame reports whether id is a frame observed within frameTTL.
func (e *engine) knownFrame(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, f := range e.frames {
		if f.id == id && time.Since(f.at) <= frameTTL {
			return true
		}
	}
	return false
}

func (e *engine) agentInput(w http.ResponseWriter, r *http.Request) {
	if !e.cfg.Control || !e.controlAllowed() {
		agentError(w, 403, "control_not_allowed", nil)
		return
	}
	var in inputRequest
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Events) < 1 || len(in.Events) > 64 {
		agentError(w, 400, "invalid_argument", nil)
		return
	}
	// The whole batch is checked before anything happens.
	for _, ev := range in.Events {
		if !ev.valid() {
			agentError(w, 400, "invalid_argument", nil)
			return
		}
	}
	if !e.knownFrame(in.FrameID) {
		agentError(w, 409, "stale_frame", nil)
		return
	}
	e.inputMu.Lock()
	defer e.inputMu.Unlock()
	defer releaseKeys()
	applied := 0
	for i, ev := range in.Events {
		if i > 0 {
			select {
			case <-r.Context().Done():
				agentError(w, 499, "cancelled", &applied)
				return
			case <-time.After(inputGap):
			}
		}
		if err := e.input(ev); err != nil {
			agentError(w, 500, "input_failed", &applied)
			return
		}
		applied++
	}
	_ = json.NewEncoder(w).Encode(map[string]int{"applied": applied})
}

// snapshot captures one still frame of the display input maps to, at most
// maxWidth wide, as JPEG or PNG.
func snapshot(ctx context.Context, maxWidth int, format string) ([]byte, error) {
	bin := ffmpegPath()
	if _, err := os.Stat(bin); err != nil {
		return nil, errors.New("encoder_missing")
	}
	in, err := screenInput(ctx, bin, "30")
	if err != nil {
		return nil, err
	}
	args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, in...)
	args = append(args, snapshotArgs(maxWidth, format)...)
	c := exec.CommandContext(ctx, bin, args...)
	hideProcess(c)
	var out limitedBuffer
	c.Stdout = &out
	c.Stderr = io.Discard
	if err := c.Run(); err != nil || out.over || out.Len() == 0 {
		return nil, errors.New("screen_capture_failed")
	}
	return out.Bytes(), nil
}

// snapshotArgs skip the first frames (AVFoundation's first one can be stale
// or black) and scale down only.
func snapshotArgs(maxWidth int, format string) []string {
	vf := fmt.Sprintf("select=gte(n\\,3),scale='min(%d,iw)':-2", maxWidth)
	args := []string{"-an", "-vf", vf, "-frames:v", "1", "-f", "image2pipe"}
	if format == "png" {
		return append(args, "-c:v", "png", "pipe:1")
	}
	return append(args, "-c:v", "mjpeg", "-pix_fmt", "yuvj420p", "-q:v", "4", "pipe:1")
}

type limitedBuffer struct {
	bytes.Buffer
	over bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > snapshotLimit {
		b.over = true
		return 0, errors.New("frame too large")
	}
	return b.Buffer.Write(p)
}
