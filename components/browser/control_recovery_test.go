package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

// Target.getTargetInfo has advanced to a redirect destination, but history
// still points at the allowed document. Restoring that same entry does not
// cancel the pending redirect: it must first be stopped explicitly.
type pendingRedirectClient struct {
	events           chan *cdp.Event
	url              string
	stopped, blanked bool
}

func (c *pendingRedirectClient) Event() <-chan *cdp.Event { return c.events }
func (c *pendingRedirectClient) Call(_ context.Context, _ string, method string, params any) ([]byte, error) {
	switch method {
	case "Target.attachToTarget":
		return []byte(`{"sessionId":"sid"}`), nil
	case "Target.getTargetInfo":
		return json.Marshal(map[string]any{"targetInfo": map[string]any{"targetId": "page", "type": "page", "url": c.url}})
	case "Page.stopLoading":
		c.stopped = true
		c.url = "https://example.com/"
	case "Page.getNavigationHistory":
		return []byte(`{"currentIndex":0,"entries":[{"id":1,"url":"https://example.com/","title":"Allowed"}]}`), nil
	case "Page.navigateToHistoryEntry":
		if c.stopped {
			c.url = "https://example.com/"
		}
	case "Page.navigate":
		raw, _ := json.Marshal(params)
		var in proto.PageNavigate
		_ = json.Unmarshal(raw, &in)
		if in.URL == "about:blank" {
			c.blanked = true
			c.url = in.URL
		}
		if c.stopped {
			c.url = in.URL
		}
		return []byte(`{"frameId":"page"}`), nil
	case "Runtime.evaluate":
		return json.Marshal(map[string]any{"result": map[string]any{"type": "object", "value": map[string]any{"ready": true, "url": c.url}}})
	case "Target.setDiscoverTargets", "Page.enable":
	default:
		return nil, fmt.Errorf("unexpected recovery call %s", method)
	}
	return []byte(`{}`), nil
}
func TestControlHostRecoveryCancelsPendingRedirectBeforeRestoring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wire := &pendingRedirectClient{events: make(chan *cdp.Event), url: "https://outside.example/redirect"}
	defer close(wire.events)
	b := rod.New().Context(ctx).ControlURL("").Client(wire).NoDefaultDevice()
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	p, err := b.PageFromTarget("page")
	if err != nil {
		t.Fatal(err)
	}
	s := &session{ctx: ctx, browser: b, pages: map[proto.TargetTargetID]*rod.Page{"page": p}, installs: map[proto.TargetTargetID]*targetInstall{"page": {sid: "sid", rootFrame: "page"}}, contexts: map[string]map[proto.RuntimeExecutionContextID]proto.PageFrameID{"sid": {1: "page"}}}
	c := &controlSession{ctx: ctx, browser: s, initialTab: "page", hosts: []string{"example.com"}, allowedURLs: map[proto.TargetTargetID]string{"page": "https://example.com/"}, wake: make(chan struct{}), frameNames: map[proto.PageFrameID]string{}}
	violation := c.enforceHosts(ctx)
	if violation == nil || violation.Code != "host_not_allowed" {
		t.Fatal("domain denial was lost", violation)
	}
	if !wire.stopped || wire.blanked || wire.url != "https://example.com/" {
		t.Fatal("pending redirect replaced the restored document", wire.stopped, wire.blanked, wire.url)
	}
}
