package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"image"
	_ "image/png"
	"strconv"
)

func (c *controlSession) screenshot(ctx context.Context, p *rod.Page, in controlInput) (map[string]any, error) {
	count := 0
	if in.Ref != "" {
		count++
	}
	if in.Clip != nil {
		count++
	}
	if in.Full {
		count++
	}
	if count > 1 {
		return nil, errors.New("invalid_argument")
	}
	n := node{ID: "n_" + randID(), Type: "screenshot", Clip: in.Clip, Full: in.Full}
	if in.Ref != "" {
		r, _, err := c.lookup(ctx, p, in.Ref)
		if err != nil {
			return nil, err
		}
		n.Target, err = c.describe(ctx, r)
		if err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	n.Into = "data" + strconv.Itoa(c.variable+1)
	c.mu.Unlock()
	if err := c.budget(n); err != nil {
		return nil, err
	}
	result, err := c.captureImage(ctx, p, in)
	if err != nil {
		return nil, err
	}
	if err = c.appendStep(n); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.variable++
	c.mu.Unlock()
	result["into"] = n.Into
	result["step_id"] = n.ID
	return result, nil
}

func (c *controlSession) captureImage(ctx context.Context, p *rod.Page, in controlInput) (map[string]any, error) {
	privacy, err := c.screenshotPrivacy(ctx, p)
	if err != nil {
		return nil, err
	}
	sid := c.browser.primarySession(p.TargetID)
	var metrics proto.PageGetLayoutMetricsResult
	if err := c.call(ctx, sid, "Page.getLayoutMetrics", nil, &metrics); err != nil {
		return nil, err
	}
	view := metrics.CSSVisualViewport
	if view == nil {
		return nil, errors.New("component_unavailable")
	}
	clip := &proto.PageViewport{X: view.PageX, Y: view.PageY, Width: view.ClientWidth, Height: view.ClientHeight, Scale: 1}
	if in.Full {
		if metrics.CSSContentSize == nil {
			return nil, errors.New("component_unavailable")
		}
		clip.X = metrics.CSSContentSize.X
		clip.Y = metrics.CSSContentSize.Y
		clip.Width = metrics.CSSContentSize.Width
		clip.Height = metrics.CSSContentSize.Height
	}
	if in.Clip != nil {
		if !in.Clip.valid() || in.Clip.X+in.Clip.Width > view.ClientWidth || in.Clip.Y+in.Clip.Height > view.ClientHeight {
			return nil, errors.New("invalid_argument")
		}
		clip.X += in.Clip.X
		clip.Y += in.Clip.Y
		clip.Width = in.Clip.Width
		clip.Height = in.Clip.Height
	}
	if in.Ref != "" {
		r, info, err := c.lookup(ctx, p, in.Ref)
		if err != nil {
			return nil, err
		}
		if info.Sensitive {
			return nil, errors.New("sensitive_snapshot")
		}
		point, err := c.elementPoint(ctx, p, r, false)
		if err != nil {
			return nil, err
		}
		clip.X = view.PageX + point.X - info.Rect.Width/2
		clip.Y = view.PageY + point.Y - info.Rect.Height/2
		clip.Width = info.Rect.Width
		clip.Height = info.Rect.Height
	}
	if clip.X < 0 || clip.Y < 0 || clip.Width <= 0 || clip.Height <= 0 || clip.Width*clip.Height > 32<<20 {
		return nil, errors.New("artifact_too_large")
	}
	if err := c.browser.setControlPanelHidden(ctx, p); err != nil {
		return nil, err
	}
	defer func() { go c.browser.updatePanel("") }()
	var shot proto.PageCaptureScreenshotResult
	if err := c.call(ctx, sid, "Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": in.Full || in.Ref != "", "clip": clip}, &shot); err != nil {
		return nil, err
	}
	if len(shot.Data) == 0 {
		return nil, errors.New("component_unavailable")
	}
	after, err := c.screenshotPrivacy(ctx, p)
	if err != nil || !sameScreenshotPrivacy(privacy, after) {
		return nil, errors.New("sensitive_snapshot")
	}
	shot.Data, err = maskScreenshot(shot.Data, privacy.Masks, view.PageX, view.PageY, clip)
	if err != nil {
		return nil, err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(shot.Data))
	if err != nil {
		return nil, errors.New("component_unavailable")
	}
	id, err := c.e.storeArtifact(c.scope, "image/png", shot.Data)
	if err != nil {
		return nil, err
	}
	return map[string]any{"screenshot": map[string]any{"id": id, "type": "image/png", "size": len(shot.Data)}, "image": map[string]any{"width": config.Width, "height": config.Height, "scale": float64(config.Width) / clip.Width}}, nil
}

// Only the captured page's top collector owns a panel. Dead advertising
// worlds and collectors in other tabs must not veto a private screenshot.
func (s *session) setControlPanelHidden(ctx context.Context, p *rod.Page) error {
	state := s.recordingPanelState("")
	state.Hide = true
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	s.mu.Lock()
	installed := s.installs[p.TargetID]
	var sid proto.TargetSessionID
	var root proto.PageFrameID
	if installed != nil {
		sid, root = installed.sid, installed.rootFrame
	}
	ids := []proto.RuntimeExecutionContextID{}
	for id, frame := range s.contexts[string(sid)] {
		if frame == root && root != "" {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	hidden := false
	for _, id := range ids {
		data, err := s.browser.Call(ctx, string(sid), "Runtime.evaluate", map[string]any{
			"expression": "(()=>{if(!window.__mycoRecorder)return false;window.__mycoRecorder.updatePanel(" + string(raw) + ");return !document.getElementById('__myco_recording_panel__')})()",
			"contextId":  id, "returnByValue": true,
		})
		if err != nil {
			controlDiagnostic("screenshot_panel_context", err)
			continue
		}
		var result proto.RuntimeEvaluateResult
		if json.Unmarshal(data, &result) == nil && result.ExceptionDetails == nil && result.Result != nil && result.Result.Value.Bool() {
			hidden = true
		}
	}
	if !hidden {
		controlDiagnostic("screenshot_panel_unavailable", nil)
		return errors.New("component_unavailable")
	}
	return nil
}
