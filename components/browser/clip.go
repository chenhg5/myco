package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Coordinates are relative to the top-level viewport in CSS pixels, not to
// the full document and not to the screenshot's device-pixel resolution.
type screenshotClip struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (c *screenshotClip) valid() bool {
	if c == nil {
		return false
	}
	for _, v := range []float64{c.X, c.Y, c.Width, c.Height, c.X + c.Width, c.Y + c.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return c.X >= 0 && c.Y >= 0 && c.Width > 0 && c.Height > 0
}
func (c *screenshotClip) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return errors.New("invalid_argument")
	}
	for _, name := range []string{"x", "y", "width", "height"} {
		if len(fields[name]) == 0 || bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
			return errors.New("invalid_argument")
		}
	}
	type plain screenshotClip
	var value plain
	if json.Unmarshal(raw, &value) != nil {
		return errors.New("invalid_argument")
	}
	*c = screenshotClip(value)
	if !c.valid() {
		return errors.New("invalid_argument")
	}
	return nil
}

// Do not let a present-but-null clip or conflicting JSON fields silently
// become an ordinary full-page/element screenshot after decoding.
func (n *node) UnmarshalJSON(raw []byte) error {
	type plain node
	var value plain
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if _, present := fields["clip"]; present {
		if value.Clip == nil || value.Type != "screenshot" {
			return errors.New("invalid_argument")
		}
		if _, present := fields["target"]; present {
			return errors.New("invalid_argument")
		}
		if _, present := fields["full"]; present {
			return errors.New("invalid_argument")
		}
	}
	*n = node(value)
	return nil
}

func (e *engine) clipSnapshot(owner string, page *rod.Page, clip *screenshotClip) (string, error) {
	if !clip.valid() {
		return "", errors.New("invalid_argument")
	}
	ctx, cancel := context.WithTimeout(page.GetContext(), 4*time.Second)
	defer cancel()
	page = page.Context(ctx)
	// The same sensitive-page check as normal evidence applies. Do not persist
	// the DOM, a thumbnail or any pixels outside the requested region.
	shape, err := page.Eval(snapshotSource)
	if err != nil {
		return "", err
	}
	if shape.Value.Get("unsafe").Bool() {
		return "", errors.New("sensitive_snapshot")
	}
	metrics, err := (proto.PageGetLayoutMetrics{}).Call(page)
	if err != nil {
		return "", err
	}
	view := metrics.CSSVisualViewport
	if view == nil || clip.X+clip.Width > view.ClientWidth || clip.Y+clip.Height > view.ClientHeight {
		return "", errors.New("invalid_argument")
	}
	result, err := (proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng, FromSurface: true, CaptureBeyondViewport: false, Clip: &proto.PageViewport{X: view.PageX + clip.X, Y: view.PageY + clip.Y, Width: clip.Width, Height: clip.Height, Scale: 1}}).Call(page)
	if err != nil {
		return "", err
	}
	if len(result.Data) == 0 {
		return "", errors.New("component_failed")
	}
	return e.storeArtifact(owner, "image/png", result.Data)
}
