package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (c *controlSession) describe(ctx context.Context, r controlRef) (*target, error) {
	id, err := c.resolve(ctx, r)
	if err != nil {
		return nil, err
	}
	defer c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
	var t target
	if err = c.function(ctx, r.sid, id, "describe", nil, &t); err != nil || len(t.Locators) == 0 {
		return nil, errors.New("target_not_found")
	}
	t.Snapshot = ""
	t.Frame, err = c.frameChain(ctx, r)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
func (c *controlSession) act(ctx context.Context, p *rod.Page, in controlInput) (map[string]any, error) {
	n := node{ID: "n_" + randID(), Type: in.Op, URL: in.URL, Value: in.Value, Key: in.Key, DY: in.DY, Wait: in.Wait, TimeoutMS: in.TimeoutMS}
	after := c.browser.cursor()
	c.browser.mu.Lock()
	oldTabs := map[proto.TargetTargetID]bool{}
	for id := range c.browser.pages {
		oldTabs[id] = true
	}
	current := c.browser.current
	c.browser.mu.Unlock()
	var prefix *node
	if current != p.TargetID && in.Op != "switch_tab" {
		c.tabName(p.TargetID)
		c.mu.Lock()
		index := 0
		for i, id := range c.tabOrder {
			if id == p.TargetID {
				index = i
				break
			}
		}
		c.mu.Unlock()
		prefix = &node{ID: "n_" + randID(), Type: "tab", Tab: strconv.Itoa(index)}
	}
	sid := c.browser.primarySession(p.TargetID)
	var r controlRef
	var info controlDOMInfo
	var err error
	required := in.Op == "click" || in.Op == "dblclick" || in.Op == "hover" || in.Op == "type" || in.Op == "select" || in.Op == "wait" && in.Wait == "element"
	if required && in.Ref == "" {
		return nil, errors.New("invalid_argument")
	}
	if in.Ref != "" {
		r, info, err = c.lookup(ctx, p, in.Ref)
		if err != nil {
			return nil, err
		}
		n.Target, err = c.describe(ctx, r)
		if err != nil {
			return nil, err
		}
	}
	switch in.Op {
	case "navigate":
		if checkedURL(in.URL) != nil {
			return nil, errors.New("invalid_argument")
		}
	case "type":
		n.Clear = in.Clear == nil || *in.Clear
		if info.Sensitive {
			return nil, errors.New("sensitive_field")
		}
	case "select":
		if info.Sensitive {
			return nil, errors.New("sensitive_field")
		}
	case "press":
		if _, _, err = keyEvents(in.Key); err != nil {
			return nil, errors.New("invalid_argument")
		}
		key := strings.ToLower(in.Key)
		if strings.Contains(key, "+v") || key == "shift+insert" {
			return nil, errors.New("permission_denied")
		}
	case "dblclick":
		n.Type = "click"
		n.Count = 2
	case "switch_tab", "close_tab":
		if in.Tab == "" {
			return nil, errors.New("invalid_argument")
		}
		n.Type = "tab"
		if in.Op == "close_tab" {
			n.Tab = "close"
		} else {
			c.mu.Lock()
			for i, id := range c.tabOrder {
				if id == p.TargetID {
					n.Tab = strconv.Itoa(i)
					break
				}
			}
			c.mu.Unlock()
		}
	case "dialog":
		if in.Choice != "accept" && in.Choice != "dismiss" {
			return nil, errors.New("invalid_argument")
		}
		n.Dialog = in.Choice
	case "wait":
		if in.Wait == "url" && in.Value == "" {
			return nil, errors.New("invalid_argument")
		}
	case "scroll":
		if math.IsNaN(in.DY) || math.IsInf(in.DY, 0) || math.Abs(in.DY) > 100000 {
			return nil, errors.New("invalid_argument")
		}
	}
	// Reserve the largest possible derived wait/popup before input. Redirects
	// update that one wait, rather than spending unbounded recording steps.
	derived := in.Op == "click" || in.Op == "dblclick" || in.Op == "press" && (in.Key == "Enter" || strings.HasSuffix(in.Key, "+Enter"))
	reserve := []node{n}
	if prefix != nil {
		reserve = append(reserve, *prefix)
	}
	if derived {
		reserve = append(reserve, node{ID: "n_reserved", Type: "wait", Wait: "url", Value: strings.Repeat("<", 8192)}, node{ID: "n_reserved", Type: "tab", Tab: "popup"})
	}
	c.mu.Lock()
	c.reservedWait = false
	c.reservedPopup = false
	c.navigationUntil = time.Time{}
	c.mu.Unlock()
	if err = c.budget(reserve...); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.navigationUntil = time.Time{}
	c.pendingURL = ""
	c.pendingPopup = false
	c.navigationNode = ""
	c.popupNode = ""
	c.pending = true
	if derived {
		c.reservedWait, c.reservedPopup = true, true
		c.navigationUntil = time.Now().Add(2 * time.Second)
		c.navigationTab = p.TargetID
	}
	c.mu.Unlock()
	committed := false
	defer func() {
		if !committed {
			c.mu.Lock()
			c.pending = false
			c.navigationUntil = time.Time{}
			c.reservedWait = false
			c.reservedPopup = false
			c.mu.Unlock()
		}
	}()
	dispatched := false
	interruptedByDialog := false
	do := func(method string, args any) error {
		if interruptedByDialog && strings.HasPrefix(method, "Input.") {
			return nil
		}
		dispatched = true
		if strings.HasPrefix(method, "Input.") {
			var err error
			interruptedByDialog, err = c.inputCall(ctx, sid, method, args, after, p.TargetID, r.sid)
			return err
		}
		return c.call(ctx, sid, method, args, nil)
	}
	focus := func() error {
		if in.Ref == "" {
			return nil
		}
		return c.call(ctx, r.sid, "DOM.focus", map[string]any{"backendNodeId": r.backend}, nil)
	}
	if prefix != nil {
		if _, err = p.Context(ctx).Activate(); err != nil {
			return nil, err
		}
		c.browser.mu.Lock()
		c.browser.current = p.TargetID
		c.browser.mu.Unlock()
		if err = c.appendStep(*prefix); err != nil {
			return nil, err
		}
	}
	switch in.Op {
	case "navigate":
		dispatched = true
		err = p.Context(ctx).Navigate(in.URL)
	case "back", "forward":
		var history proto.PageGetNavigationHistoryResult
		err = c.call(ctx, sid, "Page.getNavigationHistory", nil, &history)
		if err == nil {
			index := history.CurrentIndex - 1
			if in.Op == "forward" {
				index = history.CurrentIndex + 1
			}
			if index < 0 || index >= len(history.Entries) {
				return nil, errors.New("target_not_found")
			}
			err = do("Page.navigateToHistoryEntry", map[string]any{"entryId": history.Entries[index].ID})
		}
	case "reload":
		err = do("Page.reload", map[string]any{})
	case "click", "dblclick", "hover":
		if info.Disabled || !info.Visible {
			return nil, errors.New("target_not_interactable")
		}
		var point *controlRect
		point, err = c.point(ctx, p, r)
		if err != nil {
			break
		}
		err = do("Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": point.X, "y": point.Y})
		if err == nil && in.Op != "hover" {
			count := 1
			if in.Op == "dblclick" {
				count = 2
			}
			for i := 1; i <= count && err == nil; i++ {
				err = do("Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "button": "left", "clickCount": i, "x": point.X, "y": point.Y})
				if err == nil {
					err = do("Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "button": "left", "clickCount": i, "x": point.X, "y": point.Y})
				}
			}
		}
	case "type", "press", "select":
		if in.Ref != "" && (info.Disabled || !info.Visible) {
			return nil, errors.New("target_not_interactable")
		}
		err = focus()
		if err != nil {
			break
		}
		if in.Ref != "" {
			err = c.verifyFocus(ctx, r)
		} else {
			err = c.checkFocus(ctx, p)
		}
		if err != nil {
			break
		}
		if in.Op == "press" {
			down, up, _ := keyEvents(in.Key)
			err = do("Input.dispatchKeyEvent", down)
			if err == nil {
				err = do("Input.dispatchKeyEvent", up)
			}
		} else if in.Op == "type" {
			if n.Clear {
				modifier := "Control+a"
				if runtime.GOOS == "darwin" {
					modifier = "Meta+a"
				}
				down, up, _ := keyEvents(modifier)
				err = do("Input.dispatchKeyEvent", down)
				if err == nil {
					err = do("Input.dispatchKeyEvent", up)
				}
				if err == nil {
					down, up, _ = keyEvents("Backspace")
					err = do("Input.dispatchKeyEvent", down)
					if err == nil {
						err = do("Input.dispatchKeyEvent", up)
					}
				}
			}
			if err == nil {
				err = c.verifyFocus(ctx, r)
			}
			if err == nil {
				err = do("Input.insertText", map[string]any{"text": in.Value})
			}
		} else {
			var id proto.RuntimeRemoteObjectID
			id, err = c.resolve(ctx, r)
			if err == nil {
				var result map[string]any
				dispatched = true
				err = c.function(ctx, r.sid, id, "select", in.Value, &result)
				_ = c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
				if result["error"] != nil {
					err = errors.New("target_not_interactable")
				}
			}
		}
	case "scroll":
		if in.Ref != "" {
			var id proto.RuntimeRemoteObjectID
			id, err = c.resolve(ctx, r)
			if err == nil {
				var result any
				dispatched = true
				err = c.function(ctx, r.sid, id, "scroll", in.DY, &result)
				_ = c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
			}
		} else {
			dispatched = true
			_, err = p.Context(ctx).Eval(`y=>{window.scrollBy(0,y);return true}`, in.DY)
		}
	case "wait":
		switch in.Wait {
		case "network":
			p.Context(ctx).WaitRequestIdle(300*time.Millisecond, nil, nil, nil)()
			err = ctx.Err()
		case "url":
			for {
				var info *proto.TargetTargetInfo
				info, err = p.Context(ctx).Info()
				if err != nil || strings.Contains(info.URL, in.Value) {
					break
				}
				select {
				case <-ctx.Done():
					err = ctx.Err()
				case <-time.After(30 * time.Millisecond):
				}
				if err != nil {
					break
				}
			}
		case "element":
			for {
				_, info, err = c.lookup(ctx, p, in.Ref)
				if err != nil || info.Visible {
					break
				}
				select {
				case <-ctx.Done():
					err = ctx.Err()
				case <-time.After(50 * time.Millisecond):
				}
				if err != nil {
					break
				}
			}
		}
	case "switch_tab":
		dispatched = true
		_, err = p.Context(ctx).Activate()
		if err == nil {
			c.browser.mu.Lock()
			c.browser.current = p.TargetID
			c.browser.mu.Unlock()
		}
	case "close_tab":
		c.mu.Lock()
		order := append([]proto.TargetTargetID{}, c.tabOrder...)
		c.mu.Unlock()
		var previous proto.TargetTargetID
		for _, id := range order {
			if id == p.TargetID {
				break
			}
			c.browser.mu.Lock()
			exists := c.browser.pages[id] != nil
			c.browser.mu.Unlock()
			if exists {
				previous = id
			}
		}
		if previous == "" {
			return nil, errors.New("target_not_found")
		}
		dispatched = true
		err = p.Context(ctx).Close()
		if err == nil {
			c.browser.mu.Lock()
			c.browser.current = previous
			p = c.browser.pages[previous]
			c.browser.mu.Unlock()
			if p != nil {
				_, err = p.Context(ctx).Activate()
			} else {
				err = errors.New("target_not_found")
			}
		}
	case "dialog":
		err = do("Page.handleJavaScriptDialog", map[string]any{"accept": in.Choice == "accept"})
	}
	if err != nil {
		c.mu.Lock()
		c.pending = false
		c.navigationUntil = time.Time{}
		c.mu.Unlock()
		if dispatched {
			return nil, errors.New("outcome_unknown")
		}
		return nil, err
	}
	c.enforceHosts(ctx)
	if c.hasHostViolation() {
		return nil, errors.New("host_not_allowed")
	}
	if in.Op == "click" || in.Op == "dblclick" || in.Op == "press" {
		if !c.dialogOpen() {
			if syncErr := c.barrier(ctx, p); syncErr == nil {
				c.mu.Lock()
				hasPopup := c.pendingPopup
				c.mu.Unlock()
				if hasPopup {
					event, e := c.browser.waitObserved(ctx, after, func(evt observation) bool { return evt.Kind == "target" && !oldTabs[evt.Target] })
					if e == nil {
						e = c.waitPopup(ctx, event.Target)
					}
					if e != nil {
						if c.hasHostViolation() {
							return nil, errors.New("host_not_allowed")
						}
						return nil, errors.New("outcome_unknown")
					}
				}
			}
		}
	}
	c.enforceHosts(ctx)
	if c.hasHostViolation() {
		return nil, errors.New("host_not_allowed")
	}
	if err = c.appendStep(n); err != nil {
		return nil, err
	}
	committed = true
	c.mu.Lock()
	c.pending = false
	pendingURL, popup := c.pendingURL, c.pendingPopup
	c.mu.Unlock()
	c.captureNavigation(p.TargetID, pendingURL, popup)
	ids := []string{}
	if prefix != nil {
		ids = append(ids, prefix.ID)
	}
	ids = append(ids, n.ID)
	newTab := false
	c.browser.mu.Lock()
	for id := range c.browser.pages {
		if !oldTabs[id] {
			newTab = true
		}
	}
	active := c.browser.pages[c.browser.current]
	c.browser.mu.Unlock()
	if active != nil {
		p = active
	}
	result := map[string]any{"ok": true, "step_ids": ids, "active_tab": c.activeTab(), "tabs": c.tabs(ctx), "new_tab": newTab}
	var snap map[string]any
	var snapErr error
	if c.dialogOpen() {
		snapErr = errors.New("target_not_interactable")
	} else {
		snap, snapErr = c.snapshot(ctx, p, "a11y", 80, 16<<10)
	}
	if snapErr != nil {
		result["snapshot_error"] = controlNativeError(snapErr)
	} else {
		result["snapshot"] = snap
	}
	if info, e := p.Context(ctx).Info(); e == nil {
		result["url"] = info.URL
	}
	return result, nil
}
func (c *controlSession) checkFocus(ctx context.Context, p *rod.Page) error {
	for _, frame := range c.framesFor(p) {
		var out struct {
			Result proto.RuntimeRemoteObject `json:"result"`
		}
		err := c.call(ctx, frame.sid, "Runtime.evaluate", map[string]any{"expression": `(()=>{let a=document.activeElement;while(a?.shadowRoot?.activeElement)a=a.shadowRoot.activeElement;for(let n=a;n;n=n.parentElement||n.getRootNode?.().host){if(n.matches?.('input[type="password"],[data-sensitive],[autocomplete="one-time-code"]')||/otp|one.?time|verification.?code/i.test(n.name||''))return true}return false})()`, "contextId": frame.context, "returnByValue": true}, &out)
		if err != nil {
			return errors.New("sensitive_field")
		}
		if out.Result.Value.Bool() {
			return errors.New("sensitive_field")
		}
		var ax proto.AccessibilityGetFullAXTreeResult
		if c.call(ctx, frame.sid, "Accessibility.getFullAXTree", map[string]any{"frameId": frame.id}, &ax) != nil {
			return errors.New("sensitive_field")
		}
		for _, n := range ax.Nodes {
			if n.BackendDOMNodeID == 0 {
				continue
			}
			for _, property := range n.Properties {
				if string(property.Name) == "focused" && property.Value != nil && property.Value.Value.Bool() {
					r := controlRef{sid: frame.sid, frame: frame.id, tab: p.TargetID, backend: n.BackendDOMNodeID, epoch: frame.epoch}
					info, err := c.inspect(ctx, r)
					if err != nil || info.Sensitive {
						return errors.New("sensitive_field")
					}
				}
			}
		}

	}
	return nil
}
func (c *controlSession) point(ctx context.Context, p *rod.Page, r controlRef) (*controlRect, error) {
	return c.elementPoint(ctx, p, r, true)
}
func (c *controlSession) elementPoint(ctx context.Context, p *rod.Page, r controlRef, scroll bool) (*controlRect, error) {
	if scroll {
		if _, err := p.Context(ctx).Activate(); err != nil {
			return nil, err
		}
	}
	// Scroll the exact backend node, then re-resolve it. No locator fallback.
	if err := func() error {
		if !scroll {
			return nil
		}
		return c.call(ctx, r.sid, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": r.backend}, nil)
	}(); err != nil {
		return nil, errors.New("stale_ref")
	}
	if scroll {
		if err := c.present(ctx, p); err != nil {
			return nil, err
		}
	}
	info, err := c.inspect(ctx, r)
	if err != nil {
		return nil, err
	}
	if !info.Visible || info.Rect == nil {
		return nil, errors.New("target_not_interactable")
	}
	x, y := info.Rect.X+info.Rect.Width/2, info.Rect.Y+info.Rect.Height/2
	frame := r.frame
	sid := r.sid
	for depth := 0; depth < 32; depth++ {
		c.browser.mu.Lock()
		parent := c.browser.frames[frame].Parent
		c.browser.mu.Unlock()
		if parent == "" {
			break
		}
		// DOM rects are local to a frame, including same-process children.
		var owner struct {
			Backend proto.DOMBackendNodeID `json:"backendNodeId"`
		}
		var parentFrame controlFrame
		found := false
		for _, f := range c.framesFor(p) {
			if f.id == parent {
				parentFrame = f
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("target_not_interactable")
		}
		if err = c.call(ctx, parentFrame.sid, "DOM.getFrameOwner", map[string]any{"frameId": frame}, &owner); err != nil {
			return nil, err
		}
		ownerRef := controlRef{sid: parentFrame.sid, frame: parent, tab: p.TargetID, backend: owner.Backend, epoch: parentFrame.epoch}
		parentInfo, e := c.inspect(ctx, ownerRef)
		if e != nil || parentInfo.Rect == nil {
			return nil, errors.New("target_not_interactable")
		}
		if !parentInfo.AxisAligned {
			return nil, errors.New("target_not_interactable")
		}
		x = parentInfo.Rect.X + (x+parentInfo.BorderX)*parentInfo.ScaleX
		y = parentInfo.Rect.Y + (y+parentInfo.BorderY)*parentInfo.ScaleY
		if scroll {
			if err = c.hitTestAt(ctx, ownerRef, map[string]float64{"x": x, "y": y}); err != nil {
				return nil, err
			}
		}
		frame = parent
		sid = parentFrame.sid
	}
	_ = sid
	if scroll {
		if err = c.hitTest(ctx, r); err != nil {
			return nil, err
		}
	}
	return &controlRect{X: x, Y: y}, nil
}
func (c *controlSession) extract(ctx context.Context, p *rod.Page, in controlInput) (map[string]any, error) {
	if in.Ref == "" || in.What != "text" && in.What != "html" && in.What != "attribute" || in.What == "attribute" && (in.Attr == "" || len(in.Attr) > 160) {
		return nil, errors.New("invalid_argument")
	}
	r, _, err := c.lookup(ctx, p, in.Ref)
	if err != nil {
		return nil, err
	}
	t, err := c.describe(ctx, r)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	into := "data" + strconv.Itoa(c.variable+1)
	c.mu.Unlock()
	n := node{ID: "n_" + randID(), Type: "extract", Target: t, What: in.What, Attr: in.Attr, Into: into}
	if err = c.budget(n); err != nil {
		return nil, err
	}
	id, err := c.resolve(ctx, r)
	if err != nil {
		return nil, err
	}
	defer c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
	var out struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	err = c.function(ctx, r.sid, id, "extract", map[string]any{"what": in.What, "attr": in.Attr}, &out)
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, errors.New(out.Error)
	}
	if err = c.appendStep(n); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.variable++
	c.mu.Unlock()
	truncated := len(out.Text) >= 49152
	for {
		encoded, _ := json.Marshal(out.Text)
		if len(encoded) <= 48<<10 {
			break
		}
		out.Text = truncate(out.Text, len([]rune(out.Text))*3/4)
		truncated = true
	}
	return map[string]any{"text": out.Text, "truncated": truncated, "into": into, "step_id": n.ID}, nil
}

// Derive frame selectors with the same bounded context as the action.
func (c *controlSession) frameChain(ctx context.Context, r controlRef) ([]string, error) {
	p := pForRef(c.browser, r)
	if p == nil {
		return nil, errors.New("stale_ref")
	}
	frames := c.framesFor(p)
	byID := map[proto.PageFrameID]controlFrame{}
	for _, f := range frames {
		byID[f.id] = f
	}
	chain := []string{}
	frame := r.frame
	for i := 0; i < 32; i++ {
		c.browser.mu.Lock()
		parent := c.browser.frames[frame].Parent
		c.browser.mu.Unlock()
		if parent == "" {
			break
		}
		f, ok := byID[parent]
		if !ok {
			return nil, errors.New("stale_ref")
		}
		var owner struct {
			Backend proto.DOMBackendNodeID `json:"backendNodeId"`
		}
		if err := c.call(ctx, f.sid, "DOM.getFrameOwner", map[string]any{"frameId": frame}, &owner); err != nil {
			return nil, errors.New("stale_ref")
		}
		ref := controlRef{sid: f.sid, frame: parent, tab: r.tab, backend: owner.Backend, epoch: f.epoch}
		id, err := c.resolve(ctx, ref)
		if err != nil {
			return nil, err
		}
		var described target
		err = c.function(ctx, f.sid, id, "describe", nil, &described)
		_ = c.call(ctx, f.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
		if err != nil || len(described.Shadow) > 0 {
			return nil, errors.New("unsupported")
		}
		selector := ""
		for _, l := range described.Locators {
			if l.Kind == "css" {
				selector = l.Value
				break
			}
		}
		if selector == "" {
			return nil, errors.New("target_not_found")
		}
		chain = append([]string{selector}, chain...)
		frame = parent
	}
	return chain, nil
}
func pForRef(s *session, r controlRef) *rod.Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pages[r.tab]
}

func (c *controlSession) hitTest(ctx context.Context, r controlRef) error {
	return c.hitTestAt(ctx, r, nil)
}
func (c *controlSession) hitTestAt(ctx context.Context, r controlRef, point map[string]float64) error {
	id, err := c.resolve(ctx, r)
	if err != nil {
		return err
	}
	defer c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
	var hit struct {
		Hit bool `json:"hit"`
	}
	if err = c.function(ctx, r.sid, id, "hit", point, &hit); err != nil {
		return err
	}
	if !hit.Hit {
		return errors.New("target_not_interactable")
	}
	return nil
}

func (c *controlSession) dialogOpen() bool {
	c.browser.mu.Lock()
	defer c.browser.mu.Unlock()
	return len(c.browser.dialogs) > 0
}
func (c *controlSession) present(ctx context.Context, p *rod.Page) error {
	for _, f := range c.framesFor(p) {
		c.browser.mu.Lock()
		installed := c.browser.installs[p.TargetID]
		var root proto.PageFrameID
		if installed != nil {
			root = installed.rootFrame
		}
		c.browser.mu.Unlock()
		if f.id != root {
			continue
		}
		return c.call(ctx, f.sid, "Runtime.evaluate", map[string]any{"expression": "new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve(true))))", "contextId": f.context, "awaitPromise": true, "returnByValue": true}, nil)
	}
	return errors.New("target_not_interactable")
}
func (c *controlSession) barrier(ctx context.Context, p *rod.Page) error {
	if err := c.waitReady(ctx, p); err != nil {
		return err
	}
	id := "b_" + randID()
	done := make(chan struct{})
	c.mu.Lock()
	c.barriers[id] = done
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.barriers, id); c.mu.Unlock() }()
	encoded, _ := json.Marshal(map[string]string{"type": "control-barrier", "id": id})
	literal, _ := json.Marshal(string(encoded))
	c.browser.mu.Lock()
	installed := c.browser.installs[p.TargetID]
	var root proto.PageFrameID
	if installed != nil {
		root = installed.rootFrame
	}
	c.browser.mu.Unlock()
	for _, f := range c.framesFor(p) {
		if f.id != root {
			continue
		}
		if err := c.call(ctx, f.sid, "Runtime.evaluate", map[string]any{"expression": "window.__mycoBrowserEvent(" + string(literal) + ")", "contextId": f.context}, nil); err != nil {
			return err
		}
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return errors.New("target_not_found")
}

// Input dispatch can wait for a JavaScript modal to close. The matching
// dialog event proves delivery; let the next explicit dialog action handle it.
func (c *controlSession) inputCall(ctx context.Context, sid proto.TargetSessionID, method string, args any, after uint64, tab proto.TargetTargetID, frameSID proto.TargetSessionID) (bool, error) {
	done := make(chan error, 1)
	go func() { done <- c.call(ctx, sid, method, args, nil) }()
	for {
		c.browser.mu.Lock()
		wake := c.browser.observed
		dialog := false
		frameTarget := c.browser.targets[frameSID]
		for _, event := range c.browser.observations {
			if event.Sequence > after && event.Kind == "dialog" && (event.Target == tab || event.Target == frameTarget) {
				dialog = true
				break
			}
		}
		c.browser.mu.Unlock()
		if dialog {
			return true, nil
		}
		select {
		case err := <-done:
			return false, err
		case <-ctx.Done():
			return false, ctx.Err()
		case <-wake:
		}
	}
}

func (c *controlSession) waitPopup(ctx context.Context, id proto.TargetTargetID) error {
	done := make(chan error, 1)
	go func() { _, err := c.browser.waitPage(ctx, id); done <- err }()
	for {
		c.mu.Lock()
		wake := c.wake
		blocked := c.violation != nil
		c.mu.Unlock()
		if blocked {
			return errors.New("host_not_allowed")
		}
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		}
	}
}

func (c *controlSession) verifyFocus(ctx context.Context, r controlRef) error {
	info, err := c.inspect(ctx, r)
	if err != nil {
		return err
	}
	if info.Sensitive {
		return errors.New("sensitive_field")
	}
	object, err := c.resolve(ctx, r)
	if err != nil {
		return err
	}
	defer c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": object}, nil)
	var focus struct {
		Safe bool `json:"safe"`
	}
	if c.function(ctx, r.sid, object, "verify_focus", nil, &focus) != nil || !focus.Safe {
		return errors.New("sensitive_field")
	}
	return nil
}
