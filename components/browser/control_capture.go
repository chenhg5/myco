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
	"strings"
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

// CDP pierces closed shadow roots. Missing documents fail closed.
func (c *controlSession) safeScreenshot(ctx context.Context, p *rod.Page) error {
	frames := c.framesFor(p)
	if len(frames) == 0 || len(c.unavailableFrames(p, frames)) > 0 {
		return errors.New("sensitive_snapshot")
	}
	seen := map[proto.PageFrameID]bool{}
	for _, f := range frames {
		seen[f.id] = true
	}
	var tree proto.PageGetFrameTreeResult
	if err := c.call(ctx, c.browser.primarySession(p.TargetID), "Page.getFrameTree", nil, &tree); err != nil {
		return errors.New("sensitive_snapshot")
	}
	var checkFrame func(*proto.PageFrameTree) bool
	checkFrame = func(t *proto.PageFrameTree) bool {
		if t == nil || t.Frame == nil || !seen[t.Frame.ID] {
			return false
		}
		for _, child := range t.ChildFrames {
			if !checkFrame(child) {
				return false
			}
		}
		return true
	}
	if !checkFrame(tree.FrameTree) {
		return errors.New("sensitive_snapshot")
	}
	checked := map[proto.TargetSessionID]bool{}
	for _, f := range frames {
		if checked[f.sid] {
			continue
		}
		checked[f.sid] = true
		var doc proto.DOMGetDocumentResult
		if c.call(ctx, f.sid, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, &doc) != nil {
			return errors.New("sensitive_snapshot")
		}
		var safe func(*proto.DOMNode) bool
		safe = func(n *proto.DOMNode) bool {
			if n == nil {
				return true
			}
			attrs := map[string]string{}
			for i := 0; i+1 < len(n.Attributes); i += 2 {
				attrs[strings.ToLower(n.Attributes[i])] = strings.ToLower(n.Attributes[i+1])
			}
			if attrs["id"] == "__myco_recording_panel__" {
				return true
			}
			if (n.NodeName == "IFRAME" || n.NodeName == "FRAME") && n.FrameID != "" && !seen[n.FrameID] {
				return false
			}
			if sensitiveAttributes(attrs) {
				return false
			}

			for _, child := range n.Children {
				if !safe(child) {
					return false
				}
			}
			for _, child := range n.ShadowRoots {
				if !safe(child) {
					return false
				}
			}
			return safe(n.ContentDocument)
		}
		if !safe(doc.Root) {
			return errors.New("sensitive_snapshot")
		}
	}
	return nil
}
func (c *controlSession) captureImage(ctx context.Context, p *rod.Page, in controlInput) (map[string]any, error) {
	if err := c.safeScreenshot(ctx, p); err != nil {
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
	if err := c.browser.setControlPanelHidden(ctx); err != nil {
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
func (s *session) setControlPanelHidden(ctx context.Context) error {
	state := s.recordingPanelState("")
	state.Hide = true
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	s.mu.Lock()
	worlds := map[proto.TargetSessionID][]proto.RuntimeExecutionContextID{}
	for sid, ids := range s.contexts {
		for id := range ids {
			worlds[proto.TargetSessionID(sid)] = append(worlds[proto.TargetSessionID(sid)], id)
		}
	}
	s.mu.Unlock()
	for sid, ids := range worlds {
		for _, id := range ids {
			_, err := s.browser.Call(ctx, string(sid), "Runtime.evaluate", map[string]any{"expression": "if(window.__mycoRecorder)window.__mycoRecorder.updatePanel(" + string(raw) + ")", "contextId": id})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
