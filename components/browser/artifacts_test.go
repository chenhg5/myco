package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

type passiveScreenshotClient struct {
	ctx   context.Context
	calls []string
}

func (c *passiveScreenshotClient) GetContext() context.Context { return c.ctx }
func (c *passiveScreenshotClient) Call(ctx context.Context, _, method string, params any) ([]byte, error) {
	c.calls = append(c.calls, method)
	switch method {
	case "DOM.getContentQuads":
		return []byte(`{"quads":[[1,1,6,1,6,6,1,6]]}`), nil
	case "Page.captureScreenshot":
		request, ok := params.(proto.PageCaptureScreenshot)
		if !ok || request.Clip == nil || request.Clip.X != 1 || request.Clip.Y != 1 || request.Clip.Width != 5 || request.Clip.Height != 5 || request.Clip.Scale != 1 || !request.FromSurface {
			return nil, errors.New("capture must use CSS-pixel Clip")
		}
		// Simulate a DPR-2 browser: five CSS pixels produce ten device pixels.
		var b bytes.Buffer
		_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil)
		return json.Marshal(map[string]any{"data": b.Bytes()})
	default:
		return nil, errors.New("evidence must not run JS, scroll, focus, or wait for animation")
	}
}
func TestElementEvidenceUsesPassiveCDPWithoutWaitingForRAF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client := &passiveScreenshotClient{ctx: ctx}
	data, err := passiveElementScreenshot(client, "element", 45)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 10 {
		t.Fatal("incorrect crop", img.Bounds())
	}
	if len(client.calls) != 2 || client.calls[0] != "DOM.getContentQuads" || client.calls[1] != "Page.captureScreenshot" {
		t.Fatal(client.calls)
	}
}

// Reject any access to Rod's shared window/helper machinery. A recorder's
// background evidence must remain passive even while its actor navigates.
type recordingEvidenceClient struct {
	events chan *cdp.Event
	calls  []string
	target bool
}

func (c *recordingEvidenceClient) Event() <-chan *cdp.Event { return c.events }
func (c *recordingEvidenceClient) Call(_ context.Context, _ string, method string, params any) ([]byte, error) {
	c.calls = append(c.calls, method)
	switch method {
	case "Target.attachToTarget":
		return []byte(`{"sessionId":"recording"}`), nil
	case "Runtime.evaluate":
		in, ok := params.(proto.RuntimeEvaluate)
		if !ok || in.Expression == "window" {
			return nil, errors.New("recording evidence touched Rod's shared JS context")
		}
		if in.ReturnByValue {
			return []byte(`{"result":{"type":"object","value":{"nodes":[{"tag":"a","role":"","parent":-1}],"unsafe":false}}}`), nil
		}
		c.target = true
		return []byte(`{"result":{"type":"object","subtype":"node","objectId":"evidence-node"}}`), nil
	case "Runtime.callFunctionOn":
		return nil, errors.New("recording evidence entered Rod's helper machinery")
	case "DOM.getContentQuads":
		return []byte(`{"quads":[[1,1,6,1,6,6,1,6]]}`), nil
	case "Page.getLayoutMetrics":
		return []byte(`{"cssContentSize":{"x":0,"y":0,"width":10,"height":10}}`), nil
	case "Page.captureScreenshot":
		var b bytes.Buffer
		_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil)
		return json.Marshal(map[string]any{"data": b.Bytes()})
	case "Target.setDiscoverTargets", "Page.enable", "Runtime.releaseObject":
		return []byte(`{}`), nil
	default:
		return nil, fmt.Errorf("unexpected passive evidence method: %s", method)
	}
}
func TestRecordingEvidenceDoesNotUseRodJSContextOrHelpers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	wire := &recordingEvidenceClient{events: make(chan *cdp.Event)}
	defer close(wire.events)
	browser := rod.New().Context(ctx).ControlURL("").Client(wire).NoDefaultDevice()
	if err := browser.Connect(); err != nil {
		t.Fatal(err)
	}
	page, err := browser.PageFromTarget("page")
	if err != nil {
		t.Fatal(err)
	}
	e := &engine{dir: t.TempDir(), artifacts: map[string]artifact{}}
	id, err := e.snapshot("fixture", page, &target{Locators: []locator{{Kind: "css", Value: "a"}}, Frame: []string{"#child"}})
	if err != nil {
		t.Fatal(err)
	}
	if !wire.target || id == "" {
		t.Fatal("target evidence was silently omitted")
	}
	stored := e.artifacts[id]
	if stored.DOM == "" || stored.Thumbnail == "" {
		t.Fatal("DOM or thumbnail evidence was lost", stored)
	}
}
