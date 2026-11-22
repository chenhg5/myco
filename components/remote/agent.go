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
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// agentActions are what /status advertises (the App registers only these).
var agentActions = []string{"remote.desktop.observe", "remote.desktop.input"}

const (
	frameTTL      = 30 * time.Second // input must refer to a frame this recent
	framesKept    = 16
	snapshotLimit = 8 << 20
	inputGap      = 15 * time.Millisecond // lets apps take each event in turn
)

type frame struct {
	id       string
	at       time.Time
	geometry [4]int // the display's origin and size when it was taken
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
		in.MaxWidth = 1440
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
	f := frame{id: "frm_" + hex.EncodeToString(id[:]), at: time.Now(), geometry: e.displayGeometry()}
	e.mu.Lock()
	e.frames = append(e.frames, f)
	if len(e.frames) > framesKept {
		e.frames = e.frames[len(e.frames)-framesKept:]
	}
	e.mu.Unlock()
	_ = json.NewEncoder(w).Encode(observation{FrameID: f.id, CapturedAt: f.at.UTC(), DisplayID: "main", Width: cfg.Width, Height: cfg.Height,
		MediaType: "image/" + in.Format, Image: data})
}

// knownFrame reports whether id is a frame observed within frameTTL, of
// the display as it still is (a resolution or arrangement change makes
// its coordinates mean somewhere else).
func (e *engine) knownFrame(id string) bool {
	e.mu.Lock()
	var found *frame
	for i := range e.frames {
		if e.frames[i].id == id && time.Since(e.frames[i].at) <= frameTTL {
			found = &e.frames[i]
		}
	}
	e.mu.Unlock()
	return found != nil && found.geometry == e.displayGeometry()
}

// personControls reports whether a person's remote control session is
// connected (or connecting): then this computer has its controller.
func (e *engine) personControls() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.controlling) > 0
}

func (e *engine) displayGeometry() [4]int {
	if e.geometry != nil {
		return e.geometry()
	}
	return displayGeometry()
}

func (e *engine) agentInput(w http.ResponseWriter, r *http.Request) {
	if !e.cfg.Control || !e.controlAllowed() {
		agentError(w, 403, "control_not_allowed", nil)
		return
	}
	if e.personControls() {
		agentError(w, 409, "computer_busy", intPtr(0))
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
	// It may have waited for the lock: check again before the first event.
	if e.personControls() {
		agentError(w, 409, "computer_busy", intPtr(0))
		return
	}
	if !e.knownFrame(in.FrameID) {
		agentError(w, 409, "stale_frame", intPtr(0))
		return
	}
	applied := 0
	for i, ev := range in.Events {
		if i > 0 {
			select {
			case <-r.Context().Done():
			case <-time.After(inputGap):
			}
		}
		if r.Context().Err() != nil {
			agentError(w, 499, "cancelled", &applied)
			return
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
// maxWidth wide, as JPEG or PNG. The bundled FFmpeg has no image encoders:
// it hands over one uncompressed BMP, encoded here.
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
	args = append(args, snapshotArgs(maxWidth)...)
	c := exec.CommandContext(ctx, bin, args...)
	hideProcess(c)
	var out limitedBuffer
	c.Stdout = &out
	c.Stderr = io.Discard
	if err := c.Run(); err != nil || out.over || out.Len() == 0 {
		return nil, errors.New("screen_capture_failed")
	}
	img, err := decodeBMP(out.Bytes())
	if err != nil {
		return nil, errors.New("screen_capture_failed")
	}
	var enc bytes.Buffer
	if format == "png" {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&enc, img)
	} else {
		err = jpeg.Encode(&enc, img, &jpeg.Options{Quality: 80})
	}
	if err != nil {
		return nil, errors.New("screen_capture_failed")
	}
	return enc.Bytes(), nil
}

// snapshotArgs skip the first frames (AVFoundation's first one can be stale
// or black), scale down only, and write one 24-bit BMP to stdout.
func snapshotArgs(maxWidth int) []string {
	vf := fmt.Sprintf("select=gte(n\\,3),scale='min(%d,iw)':-2", maxWidth)
	return []string{"-an", "-vf", vf, "-frames:v", "1", "-c:v", "bmp", "-pix_fmt", "bgr24", "-f", "image2pipe", "pipe:1"}
}

// decodeBMP reads the uncompressed 24-bit BMP FFmpeg writes.
func decodeBMP(b []byte) (*image.RGBA, error) {
	if len(b) < 54 || b[0] != 'B' || b[1] != 'M' {
		return nil, errors.New("not a bmp")
	}
	off := int(binary.LittleEndian.Uint32(b[10:]))
	w := int(int32(binary.LittleEndian.Uint32(b[18:])))
	h := int(int32(binary.LittleEndian.Uint32(b[22:])))
	bpp := binary.LittleEndian.Uint16(b[28:])
	comp := binary.LittleEndian.Uint32(b[30:])
	topDown := h < 0
	if topDown {
		h = -h
	}
	if bpp != 24 || comp != 0 || w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return nil, errors.New("unsupported bmp")
	}
	stride := (w*3 + 3) &^ 3
	if off < 54 || off+stride*h > len(b) {
		return nil, errors.New("short bmp")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := h - 1 - y
		if topDown {
			row = y
		}
		src := b[off+row*stride:]
		dst := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = src[x*3+2], src[x*3+1], src[x*3], 0xff
		}
	}
	return img, nil
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

func intPtr(n int) *int { return &n }
