package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/go-rod/rod/lib/proto"
	"image"
	"image/jpeg"
	"testing"
	"time"
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
