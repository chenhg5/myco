package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type replayError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Reason    string `json:"reason,omitempty"`
	ErrorNode string `json:"error_node,omitempty"`
}

func replayErrorDTO(failure *controlError) *replayError {
	return &replayError{Code: failure.Code, Message: "browser replay: " + strings.ReplaceAll(failure.Code, "_", " "), Reason: failure.Reason, ErrorNode: failure.ErrorNode}
}

// replay shares the live controller and its safety checks. It neither creates
// an approved M2 run nor writes steps/evidence into the manual draft.
func (c *controlSession) replay(ctx context.Context, p *rod.Page, in controlInput) (map[string]any, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var graph BrowserReplayGraph
	if err := json.Unmarshal(in.Graph, &graph); err != nil {
		return nil, errors.New("invalid_argument")
	}
	nodes := map[string]node{}
	next := map[string]string{}
	incoming := map[string]bool{}
	for _, raw := range graph.Nodes {
		var n node
		if json.Unmarshal(raw, &n) != nil {
			return nil, errors.New("invalid_argument")
		}
		nodes[n.ID] = n
	}
	for _, edge := range graph.Edges {
		next[edge[0]] = edge[1]
		incoming[edge[1]] = true
	}
	start := ""
	for id := range nodes {
		if !incoming[id] {
			start = id
		}
	}
	c.mu.Lock()
	c.replaying = true
	c.replayAbort = func() { cancel(errors.New("host_not_allowed")) }
	c.last = time.Now()
	c.clearReplayRefsLocked()
	tabs := append([]proto.TargetTargetID{}, c.tabOrder...)
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.clearReplayRefsLocked()
		c.replaying = false
		c.replayAbort = nil
		c.last = time.Now()
		// Late renderer events still enforce host scope, but never enter the draft.
		c.pending = false
		c.navigationUntil = time.Time{}
		c.reservedWait = false
		c.reservedPopup = false
		c.navigationPending = false
		c.navigationLoads = nil
		c.navigationTab = ""
		c.navigationTracked = nil
		c.navigationRequests = nil
		c.pendingURL = ""
		c.pendingPopupURL = ""
		c.pendingPopup = false
	}()
	c.browser.mu.Lock()
	c.browser.current = p.TargetID
	c.browser.mu.Unlock()
	runner := &playRun{session: c.browser, scope: c.scope, tabOrder: tabs, tabs: map[string]proto.TargetTargetID{}, values: map[string]string{}}
	steps := []BrowserReplayStep{}
	output := map[string]any{}
	result := map[string]any{"ok": true, "state": "succeeded", "steps": steps, "output": output, "url": ""}
	for id := start; id != ""; id = next[id] {
		n := nodes[id]
		began := time.Now()
		if failure := c.check(false); failure != nil {
			return nil, errors.New(failure.Code)
		}
		var value any
		var stepErr error
		deniedHost := ""
		limit := 30 * time.Second
		if n.Type == "wait" && n.Wait == "time" {
			limit = time.Duration(n.MS)*time.Millisecond + time.Second
		}
		if n.TimeoutMS > 0 {
			limit = time.Duration(n.TimeoutMS) * time.Millisecond
		}
		attempts := 1
		if strings.HasPrefix(n.OnError, "retry:") {
			count, _ := strconv.Atoi(strings.TrimPrefix(n.OnError, "retry:"))
			attempts += count
		}
		for attempt := 0; attempt < attempts; attempt++ {
			if ctx.Err() != nil {
				stepErr = ctx.Err()
				if cause := context.Cause(ctx); cause != nil {
					stepErr = cause
				}
				break
			}
			p, stepErr = c.browser.page()
			if stepErr != nil {
				break
			}
			if n.Type != "tab" {
				runner.after = c.browser.cursor()
				runner.anchorTarget = p.TargetID
				runner.anchorFrame = p.FrameID
			}
			stepCtx, stop := context.WithTimeout(ctx, limit)
			value, stepErr = c.replayNode(stepCtx, p, runner, n)
			c.enforceHosts(stepCtx)
			stop()
			if violation := c.takeHostViolation(); violation != nil {
				stepErr = errors.New("host_not_allowed")
				deniedHost = violation.Host
			}
			if stepErr == nil || replayTerminalCode(controlNativeError(stepErr).Code) {
				break
			}
		}
		if cause := context.Cause(ctx); cause != nil {
			stepErr = cause
		}
		if violation := c.takeHostViolation(); violation != nil {
			stepErr = errors.New("host_not_allowed")
			deniedHost = violation.Host
		}
		step := BrowserReplayStep{Node: n.ID, State: "ok", MS: time.Since(began).Milliseconds()}
		if stepErr != nil {
			failure := controlNativeError(stepErr)
			if deniedHost != "" {
				failure.Host = deniedHost
				result["host"] = deniedHost
			}
			step.State = "failed"
			step.Error = failure.Code
			if n.OnError == "skip" && !replayTerminalCode(failure.Code) && ctx.Err() == nil {
				step.State = "skipped"
			} else {
				result["ok"], result["state"], result["error_node"], result["error"] = false, "failed", n.ID, replayErrorDTO(failure)
			}
		} else if n.Into != "" {
			candidate := map[string]any{}
			for key, item := range output {
				candidate[key] = item
			}
			runner.outputCut = false
			candidate[n.Into] = runner.cleanOutput(value, 0)
			if runner.outputCut {
				candidate["truncated"] = true
			}
			if raw, err := json.Marshal(candidate); err == nil && len(raw) <= (16<<10)-32 {
				output = candidate
				result["output"] = output
			} else {
				output["truncated"] = true
			}
		}
		steps = append(steps, step)
		result["steps"] = steps
		if result["ok"] == false {
			break
		}
	}
	if ctx.Err() != nil && result["ok"] == true {
		result["ok"], result["state"], result["error"] = false, "failed", replayErrorDTO(controlNativeError(context.Cause(ctx)))
	}
	metadata := ctx
	if active, err := c.browser.page(); err == nil {
		if info, e := active.Context(metadata).Info(); e == nil && controlHostAllowed(c.hosts, info.URL) {
			result["url"] = truncate(info.URL, 8192)
		}
	}
	resultTabs := c.tabs(metadata)
	for _, tab := range resultTabs {
		if raw, _ := tab["url"].(string); !controlHostAllowed(c.hosts, raw) {
			tab["url"], tab["title"] = "", ""
		}
	}
	result["tabs"] = resultTabs
	return result, nil
}
func (c *controlSession) clearReplayRefsLocked() {
	c.refs = map[string]controlRef{}
	c.refKeys = map[controlRef]string{}
	// refNext remains monotonic so a pre-replay reference cannot be reissued.
}
func replayTerminalCode(code string) bool {
	switch code {
	case "host_not_allowed", "outcome_unknown", "session_closed", "session_expired", "cancelled", "permission_denied", "artifact_limit", "artifact_too_large", "component_unavailable", "storage_failed":
		return true
	}
	return false
}

func (c *controlSession) replayTarget(ctx context.Context, p *rod.Page, t *target) (string, controlDOMInfo, error) {
	if t == nil {
		return "", controlDOMInfo{}, errors.New("target_not_found")
	}
	frame, err := targetPage(ctx, p, *t)
	if err != nil {
		return "", controlDOMInfo{}, err
	}
	elements, err := frame.ElementsByJS(rod.Eval(locatorSource, *t, true))
	if err != nil {
		return "", controlDOMInfo{}, err
	}
	if len(elements) > 1 {
		return "", controlDOMInfo{}, errors.New("target_not_interactable")
	}
	if len(elements) == 0 {
		return "", controlDOMInfo{}, errors.New("target_not_found")
	}
	el := elements[0]
	defer el.Release()
	described, err := el.Describe(0, false)
	if err != nil {
		return "", controlDOMInfo{}, err
	}
	fid := frame.FrameID
	if fid == "" {
		c.browser.mu.Lock()
		if installed := c.browser.installs[p.TargetID]; installed != nil {
			fid = installed.rootFrame
		}
		c.browser.mu.Unlock()
	}
	for _, ready := range c.framesFor(p) {
		if ready.id != fid || ready.sid != frame.SessionID {
			continue
		}
		ref := controlRef{sid: ready.sid, frame: ready.id, tab: p.TargetID, backend: described.BackendNodeID, epoch: ready.epoch}
		info, err := c.inspect(ctx, ref)
		if err != nil {
			return "", info, err
		}
		ref.generation = info.Generation
		ref.bound = true
		id := c.ref(ref)
		if id == "" {
			return "", info, errors.New("stale_ref")
		}
		return id, info, nil
	}
	return "", controlDOMInfo{}, errors.New("target_not_found")
}
func (c *controlSession) replayNode(ctx context.Context, p *rod.Page, runner *playRun, n node) (any, error) {
	c.enforceHosts(ctx)
	if c.hasHostViolation() {
		return nil, errors.New("host_not_allowed")
	}
	if n.Type != "dialog" && c.dialogOpen() {
		return nil, errors.New("target_not_interactable")
	}
	if n.Type == "navigate" || n.Type == "tab" && n.Tab == "new" && n.URL != "" {
		if !controlHostAllowed(c.hosts, n.URL) {
			c.recordHostViolation(c.hostViolation(n.URL))
			return nil, errors.New("host_not_allowed")
		}
	}
	var ref string
	var info controlDOMInfo
	var err error
	if n.Target != nil {
		for {
			if c.hasHostViolation() {
				return nil, errors.New("host_not_allowed")
			}
			ref, info, err = c.replayTarget(ctx, p, n.Target)
			if n.Type == "assert" && n.Check == "hidden" {
				if err != nil && err.Error() == "target_not_found" {
					err = nil
					ref = ""
					break
				}
				if err == nil && !info.Visible {
					break
				}
			} else if err == nil && info.Visible {
				break
			}
			if err != nil && err.Error() != "target_not_found" && err.Error() != "stale_ref" {
				return nil, err
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	switch n.Type {
	case "note":
		return nil, nil
	case "wait":
		if n.Wait == "time" {
			timer := time.NewTimer(time.Duration(n.MS) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
				return nil, nil
			}
		}
	case "assert":
		passed := false
		switch n.Check {
		case "hidden":
			passed = ref == "" || !info.Visible
		case "visible":
			passed = ref != "" && info.Visible
		case "text":
			value, e := c.replayExtract(ctx, p, ref, "text", "")
			if e != nil {
				return nil, e
			}
			text, _ := value.(string)
			passed = strings.Contains(text, n.Value)
		case "url":
			value, e := p.Context(ctx).Info()
			if e != nil {
				return nil, e
			}
			passed = strings.Contains(value.URL, n.Value)
		}
		if !passed {
			return nil, errors.New("assertion_failed")
		}
		return nil, nil
	case "extract":
		return c.replayExtract(ctx, p, ref, n.What, n.Attr)
	case "screenshot":
		return c.captureImage(ctx, p, controlInput{Ref: ref, Clip: n.Clip, Full: n.Full})
	case "tab":
		if n.Tab == "new" {
			c.browser.mu.Lock()
			count := len(c.browser.pages)
			c.browser.mu.Unlock()
			if count >= 32 {
				return nil, errors.New("target_not_interactable")
			}
		}
		value, err := runner.tab(ctx, n)
		if err == nil {
			if active, e := c.browser.page(); e == nil {
				c.tabName(active.TargetID)
			}
			c.mu.Lock()
			runner.tabOrder = append([]proto.TargetTargetID{}, c.tabOrder...)
			c.mu.Unlock()
		}
		return value, err
	}
	in := controlInput{Op: n.Type, Ref: ref, URL: n.URL, Value: n.Value, Key: n.Key, DY: n.DY, Wait: n.Wait, Choice: n.Dialog, Tab: c.tabName(p.TargetID)}
	if n.Type == "type" {
		in.Clear = &n.Clear
	}
	if n.Type == "click" && n.Count == 2 {
		in.Op = "dblclick"
	}
	if n.Type == "scroll" && n.Value == "absolute" {
		var position struct {
			Y float64 `json:"y"`
		}
		if ref != "" {
			r, _, err := c.lookup(ctx, p, ref)
			if err != nil {
				return nil, err
			}
			id, err := c.resolve(ctx, r)
			if err != nil {
				return nil, err
			}
			defer c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
			if err = c.function(ctx, r.sid, id, "scroll_position", nil, &position); err != nil {
				return nil, err
			}
		} else {
			v, err := p.Context(ctx).Eval(`()=>scrollY`)
			if err != nil {
				return nil, err
			}
			position.Y = v.Value.Num()
		}
		in.DY -= position.Y
	}
	value, err := c.act(ctx, p, in)
	return value, err
}
func (c *controlSession) replayExtract(ctx context.Context, p *rod.Page, ref, what, attr string) (any, error) {
	r, info, err := c.lookup(ctx, p, ref)
	if err != nil {
		return nil, err
	}
	if info.Sensitive {
		return "[redacted]", nil
	}
	id, err := c.resolve(ctx, r)
	if err != nil {
		return nil, err
	}
	defer c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
	var out struct {
		Text  string `json:"text"`
		Data  any    `json:"data"`
		Error string `json:"error"`
	}
	if err = c.function(ctx, r.sid, id, "extract", map[string]string{"what": what, "attr": attr}, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, errors.New(out.Error)
	}
	if what == "table" {
		return out.Data, nil
	}
	return out.Text, nil
}
