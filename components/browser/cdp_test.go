package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

// A paused popup cannot complete a second attachment or evaluate JS until
// runIfWaitingForDebugger. This reproduces window.open's bootstrap ordering
// without starting a real browser in ordinary unit tests.
type pausedPopupClient struct {
	mu                                    sync.Mutex
	events                                chan *cdp.Event
	binding, preload, autoAttach, resumed bool
}

func (c *pausedPopupClient) Event() <-chan *cdp.Event { return c.events }
func (c *pausedPopupClient) Call(ctx context.Context, sid, method string, params any) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch method {
	case "Runtime.addBinding":
		c.binding = true
	case "Page.addScriptToEvaluateOnNewDocument":
		c.preload = true
	case "Target.setAutoAttach":
		c.autoAttach = true
	case "Runtime.runIfWaitingForDebugger":
		if !c.binding || !c.preload || !c.autoAttach {
			return nil, errors.New("resumed before collector installation")
		}
		c.resumed = true
	case "Target.attachToTarget", "Runtime.evaluate":
		if !c.resumed {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if method == "Target.attachToTarget" {
			return []byte(`{"sessionId":"rod"}`), nil
		}
	case "Page.getFrameTree":
		return []byte(`{"frameTree":{"frame":{"id":"popup","url":"about:blank"}}}`), nil
	case "Page.createIsolatedWorld":
		return []byte(`{"executionContextId":1}`), nil
	}
	return []byte(`{}`), nil
}
func TestPausedPopupBootstrapsBeforeRodAttachment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client := &pausedPopupClient{events: make(chan *cdp.Event)}
	defer close(client.events)
	browser := rod.New().Context(ctx).ControlURL("").Client(client).NoDefaultDevice()
	if err := browser.Connect(); err != nil {
		t.Fatal(err)
	}
	s := &session{ctx: ctx, browser: browser, pages: map[proto.TargetTargetID]*rod.Page{}, installs: map[proto.TargetTargetID]*targetInstall{}, targets: map[proto.TargetSessionID]proto.TargetTargetID{}, contexts: map[string]map[proto.RuntimeExecutionContextID]proto.PageFrameID{}}
	if err := s.installTarget(proto.TargetAttachedToTarget{SessionID: "primary", TargetInfo: &proto.TargetTargetInfo{TargetID: "popup", Type: "page"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.page(); err != nil {
		t.Fatal(err)
	}
}
