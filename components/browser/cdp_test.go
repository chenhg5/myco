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
	attaches, detaches                    int
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
	case "Target.detachFromTarget":
		c.detaches++
	case "Target.attachToTarget", "Runtime.evaluate":
		if !c.resumed {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if method == "Target.attachToTarget" {
			c.attaches++
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

func TestSecondaryAttachmentResumesAfterConfigurationWithoutWaitingForRod(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client := &pausedPopupClient{events: make(chan *cdp.Event), binding: true, preload: true, autoAttach: true}
	defer close(client.events)
	browser := rod.New().Context(ctx).ControlURL("").Client(client).NoDefaultDevice()
	if err := browser.Connect(); err != nil {
		t.Fatal(err)
	}
	configured := make(chan struct{})
	close(configured)
	// ready remains open: the primary is inside Rod's second attachment,
	// which itself cannot finish until that secondary debugger is resumed.
	s := &session{ctx: ctx, browser: browser, installs: map[proto.TargetTargetID]*targetInstall{"popup": {configured: configured, ready: make(chan struct{})}}, targets: map[proto.TargetSessionID]proto.TargetTargetID{}}
	if err := s.installTarget(proto.TargetAttachedToTarget{SessionID: "secondary", TargetInfo: &proto.TargetTargetInfo{TargetID: "popup", Type: "page"}}); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	resumed := client.resumed
	client.mu.Unlock()
	if !resumed {
		t.Fatal("secondary debugger left the popup paused")
	}
}

func TestSecondaryAttachmentDoesNotResumeBeforeCollectorConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &pausedPopupClient{events: make(chan *cdp.Event), binding: true, preload: true, autoAttach: true}
	defer close(client.events)
	browser := rod.New().Context(ctx).ControlURL("").Client(client).NoDefaultDevice()
	s := &session{ctx: ctx, browser: browser, installs: map[proto.TargetTargetID]*targetInstall{"popup": {configured: make(chan struct{}), ready: make(chan struct{})}}, targets: map[proto.TargetSessionID]proto.TargetTargetID{}}
	if err := s.installTarget(proto.TargetAttachedToTarget{SessionID: "secondary", TargetInfo: &proto.TargetTargetInfo{TargetID: "popup", Type: "page"}}); !errors.Is(err, context.Canceled) {
		t.Fatal("unconfigured attachment did not cancel", err)
	}
	client.mu.Lock()
	resumed := client.resumed
	client.mu.Unlock()
	if resumed {
		t.Fatal("resumed before primary collector configuration")
	}
}

func TestRodReusesPrimarySessionAndCannotDetachIt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	wire := &pausedPopupClient{events: make(chan *cdp.Event)}
	defer close(wire.events)
	s := &session{ctx: ctx, pages: map[proto.TargetTargetID]*rod.Page{}, installs: map[proto.TargetTargetID]*targetInstall{}, targets: map[proto.TargetSessionID]proto.TargetTargetID{}, contexts: map[string]map[proto.RuntimeExecutionContextID]proto.PageFrameID{}}
	browser := rod.New().Context(ctx).ControlURL("").Client(&sessionClient{base: wire, owner: s}).NoDefaultDevice()
	s.browser = browser
	if err := browser.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := s.installTarget(proto.TargetAttachedToTarget{SessionID: "primary", TargetInfo: &proto.TargetTargetInfo{TargetID: "popup", Type: "page"}}); err != nil {
		t.Fatal(err)
	}
	page, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	if page.SessionID != "primary" {
		t.Fatalf("Rod got %s, want primary", page.SessionID)
	}
	if err := (proto.TargetDetachFromTarget{SessionID: page.SessionID}).Call(browser); err != nil {
		t.Fatal(err)
	}
	wire.mu.Lock()
	attaches, detaches := wire.attaches, wire.detaches
	wire.mu.Unlock()
	if attaches != 0 || detaches != 0 {
		t.Fatalf("wire received %d attaches and %d detaches", attaches, detaches)
	}
	if _, err := browser.Call(ctx, "", "Browser.close", map[string]any{}); err == nil {
		t.Fatal("Browser.close was allowed")
	}
}

func TestRodCannotReuseFailedCollectorSession(t *testing.T) {
	ready, configured := make(chan struct{}), make(chan struct{})
	close(ready)
	close(configured)
	failure := errors.New("collector installation failed")
	s := &session{installs: map[proto.TargetTargetID]*targetInstall{"popup": {sid: "primary", ready: ready, configured: configured, err: failure}}}
	wire := &pausedPopupClient{events: make(chan *cdp.Event)}
	client := &sessionClient{base: wire, owner: s}
	_, err := client.Call(context.Background(), "", "Target.attachToTarget", proto.TargetAttachToTarget{TargetID: "popup"})
	if !errors.Is(err, failure) {
		t.Fatal("failed session reused", err)
	}
}

type waitForContextClient struct{}

func (waitForContextClient) Event() <-chan *cdp.Event { return nil }
func (waitForContextClient) Call(ctx context.Context, _, _ string, _ any) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestRootPageRPCStillObeysTheActiveOperationContext(t *testing.T) {
	s := &session{}
	step, cancel := context.WithCancel(context.Background())
	defer cancel()
	restore := s.commands(step)
	defer restore()
	client := &sessionClient{base: waitForContextClient{}, owner: s}
	done := make(chan error, 1)
	go func() {
		_, err := client.Call(context.Background(), "primary", "Runtime.callFunctionOn", nil)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("root Page RPC ignored operation cancellation")
	}
}

type contextCheckClient struct{}

func (contextCheckClient) Event() <-chan *cdp.Event { return nil }
func (contextCheckClient) Call(ctx context.Context, _, _ string, _ any) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []byte(`{}`), nil
}
func TestActionCancellationDoesNotCancelCollectorInstallation(t *testing.T) {
	s := &session{ctx: context.Background()}
	wire := &sessionClient{base: contextCheckClient{}, owner: s}
	s.browser = rod.New().Context(s.ctx).ControlURL("").Client(wire)
	action, cancel := context.WithCancel(context.Background())
	restore := s.commands(action)
	defer restore()
	cancel()
	// The engine's installation remains governed by its own bounded context.
	if err := s.call("primary", "Page.enable", map[string]any{}, nil); err != nil {
		t.Fatal("collector inherited action cancellation", err)
	}
	// A normal Rod helper still inherits cancellation even if it used root ctx.
	if _, err := wire.Call(context.Background(), "primary", "Runtime.callFunctionOn", nil); !errors.Is(err, context.Canceled) {
		t.Fatal("Rod helper escaped action cancellation", err)
	}
}
