package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

//go:embed control_dom.js
var controlDOM string

type controlRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type controlNode struct {
	ID          string       `json:"id"`
	Parent      string       `json:"parent,omitempty"`
	Ref         string       `json:"ref,omitempty"`
	Frame       string       `json:"frame"`
	Role        string       `json:"role"`
	Name        string       `json:"name"`
	Text        string       `json:"text,omitempty"`
	Value       string       `json:"value,omitempty"`
	Interactive bool         `json:"interactive"`
	Disabled    bool         `json:"disabled"`
	Redacted    bool         `json:"redacted"`
	Rect        *controlRect `json:"rect,omitempty"`
	InViewport  bool         `json:"in_viewport"`
}
type controlDOMInfo struct {
	Generation  uint64       `json:"generation"`
	NodeType    int          `json:"node_type"`
	ScaleX      float64      `json:"scale_x"`
	ScaleY      float64      `json:"scale_y"`
	AxisAligned bool         `json:"axis_aligned"`
	BorderX     float64      `json:"border_x"`
	BorderY     float64      `json:"border_y"`
	Connected   bool         `json:"connected"`
	Panel       bool         `json:"panel"`
	Sensitive   bool         `json:"sensitive"`
	Unsafe      bool         `json:"unsafe"`
	Disabled    bool         `json:"disabled"`
	Visible     bool         `json:"visible"`
	InViewport  bool         `json:"in_viewport"`
	Interactive bool         `json:"interactive"`
	Rect        *controlRect `json:"rect"`
	Text        string       `json:"text"`
	Value       string       `json:"value"`
}
type controlFrame struct {
	id      proto.PageFrameID
	sid     proto.TargetSessionID
	epoch   uint64
	context proto.RuntimeExecutionContextID
	name    string
}

func (c *controlSession) call(ctx context.Context, sid proto.TargetSessionID, method string, in, out any) error {
	raw, err := c.browser.browser.Call(ctx, string(sid), method, in)
	if err != nil {
		return err
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}
func (c *controlSession) frameName(id proto.PageFrameID) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frameNames[id] == "" {
		c.frameNames[id] = "f" + strconv.Itoa(len(c.frameNames)+1)
	}
	return c.frameNames[id]
}
func (c *controlSession) framesFor(p *rod.Page) []controlFrame {
	s := c.browser
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []controlFrame{}
	seen := map[proto.PageFrameID]bool{}
	root := proto.PageFrameID("")
	if installed := s.installs[p.TargetID]; installed != nil {
		root = installed.rootFrame
	}
	for sid, worlds := range s.contexts {
		for contextID, frame := range worlds {
			top := frame
			visited := map[proto.PageFrameID]bool{}
			for s.frames[top].Parent != "" && !visited[top] {
				visited[top] = true
				top = s.frames[top].Parent
			}
			if top != root || seen[frame] {
				continue
			}
			seen[frame] = true
			c.mu.Lock()
			epoch := c.epochs[frame]
			c.mu.Unlock()
			out = append(out, controlFrame{id: frame, sid: proto.TargetSessionID(sid), context: contextID, epoch: epoch, name: c.frameName(frame)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].id == root {
			return true
		}
		if out[j].id == root {
			return false
		}
		return out[i].name < out[j].name
	})
	return out
}
func (c *controlSession) resolve(ctx context.Context, r controlRef) (proto.RuntimeRemoteObjectID, error) {
	c.mu.Lock()
	current := c.epochs[r.frame]
	c.mu.Unlock()
	if current != r.epoch {
		return "", errors.New("stale_ref")
	}
	c.browser.mu.Lock()
	worlds := map[proto.RuntimeExecutionContextID]proto.PageFrameID{}
	for id, f := range c.browser.contexts[string(r.sid)] {
		worlds[id] = f
	}
	c.browser.mu.Unlock()
	for id, f := range worlds {
		if f != r.frame {
			continue
		}
		var out struct {
			Object proto.RuntimeRemoteObject `json:"object"`
		}
		if c.call(ctx, r.sid, "DOM.resolveNode", map[string]any{"backendNodeId": r.backend, "executionContextId": id}, &out) == nil && out.Object.ObjectID != "" {
			return out.Object.ObjectID, nil
		}
	}
	return "", errors.New("stale_ref")
}
func (c *controlSession) function(ctx context.Context, sid proto.TargetSessionID, id proto.RuntimeRemoteObjectID, op string, value any, out any) error {
	args := []map[string]any{{"value": op}, {"value": value}}
	var reply struct {
		Result    proto.RuntimeRemoteObject `json:"result"`
		Exception any                       `json:"exceptionDetails"`
	}
	if err := c.call(ctx, sid, "Runtime.callFunctionOn", map[string]any{"objectId": id, "functionDeclaration": controlDOM, "arguments": args, "returnByValue": true}, &reply); err != nil {
		return err
	}
	if reply.Exception != nil {
		return errors.New("target_not_interactable")
	}
	raw, _ := json.Marshal(reply.Result.Value.Val())
	return json.Unmarshal(raw, out)
}
func (c *controlSession) inspect(ctx context.Context, r controlRef) (controlDOMInfo, error) {
	var info controlDOMInfo
	id, err := c.resolve(ctx, r)
	if err != nil {
		return info, err
	}
	defer c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": id}, nil)
	err = c.function(ctx, r.sid, id, "inspect", nil, &info)
	if err == nil && (!info.Connected || info.Panel || r.bound && r.generation != info.Generation) {
		err = errors.New("stale_ref")
	}
	return info, err
}
func (c *controlSession) ref(r controlRef) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epochs[r.frame] != r.epoch || c.closed {
		return ""
	}
	if id := c.refKeys[r]; id != "" {
		return id
	}
	c.refNext++
	id := "r" + strconv.FormatUint(c.refNext, 10)
	c.refKeys[r] = id
	c.refs[id] = r
	return id
}
func (c *controlSession) lookup(ctx context.Context, p *rod.Page, id string) (controlRef, controlDOMInfo, error) {
	c.mu.Lock()
	r, ok := c.refs[id]
	c.mu.Unlock()
	if !ok || r.tab != p.TargetID {
		return r, controlDOMInfo{}, errors.New("stale_ref")
	}
	info, err := c.inspect(ctx, r)
	return r, info, err
}
func axString(v *proto.AccessibilityAXValue) string {
	if v == nil {
		return ""
	}
	return v.Value.Str()
}
func (c *controlSession) snapshot(ctx context.Context, p *rod.Page, mode string, max, bytes int) (map[string]any, error) {
	if err := c.waitReady(ctx, p); err != nil {
		return nil, err
	}
	if max == 0 {
		max = 400
	}
	frames := c.framesFor(p)
	if len(frames) == 0 {
		return nil, errors.New("target_not_found")
	}
	nodes := []controlNode{}
	frameResults := []map[string]any{}
	truncated := false
	layouts := map[proto.TargetSessionID]*proto.DOMSnapshotCaptureSnapshotResult{}
	for _, frame := range frames {
		if _, exists := layouts[frame.sid]; !exists {
			var raw proto.DOMSnapshotCaptureSnapshotResult
			if c.call(ctx, frame.sid, "DOMSnapshot.captureSnapshot", map[string]any{"computedStyles": []string{"cursor"}}, &raw) == nil {
				layouts[frame.sid] = &raw
			} else {
				layouts[frame.sid] = nil
			}
		}
		layout := c.layout(ctx, frame, layouts[frame.sid])
		var tree proto.AccessibilityGetFullAXTreeResult
		err := c.call(ctx, frame.sid, "Accessibility.getFullAXTree", map[string]any{"frameId": frame.id, "depth": 32}, &tree)
		status := "ready"
		if err != nil {
			status = "unavailable"
		}
		frameResults = append(frameResults, map[string]any{"id": frame.name, "status": status})
		if err != nil {
			continue
		}
		sort.SliceStable(tree.Nodes, func(i, j int) bool {
			return layout[tree.Nodes[i].BackendDOMNodeID].visible && !layout[tree.Nodes[j].BackendDOMNodeID].visible
		})
		seen := map[proto.DOMBackendNodeID]bool{}
		for index, a := range tree.Nodes {
			if index >= 1600 {
				truncated = true
				break
			}
			if a.Ignored || a.BackendDOMNodeID == 0 {
				continue
			}
			r := controlRef{sid: frame.sid, frame: frame.id, tab: p.TargetID, backend: a.BackendDOMNodeID, epoch: frame.epoch}
			info, err := c.inspect(ctx, r)
			if err != nil {
				continue
			}
			// No input, password or sensitive ancestor values are ever taken directly
			// from AX. Inspect the live DOM in the matching isolated document first.
			n := controlNode{ID: frame.name + "n" + string(a.NodeID), Frame: frame.name, Role: axString(a.Role), Name: truncate(axString(a.Name), 512), Interactive: info.Interactive, Disabled: info.Disabled, Redacted: info.Sensitive, Rect: info.Rect, InViewport: info.InViewport}
			if a.ParentID != "" {
				n.Parent = frame.name + "n" + string(a.ParentID)
			}
			if info.Sensitive {
				n.Name = "[redacted]"
			} else {
				n.Value = truncate(info.Value, 1024)
				if n.Role == "StaticText" || n.Role == "heading" {
					n.Text = truncate(axString(a.Name), 1024)
				}
			}
			if info.NodeType == 1 {
				r.generation = info.Generation
				r.bound = true
				n.Ref = c.ref(r)
				if n.Ref == "" {
					return nil, errors.New("stale_ref")
				}
			} else {
				n.Interactive = false
			}
			if layout[a.BackendDOMNodeID].pointer && info.NodeType == 1 {
				n.Interactive = true
				if n.Name == "" {
					if pointer, ok := c.pointerNode(ctx, p.TargetID, frame, a.BackendDOMNodeID); ok {
						n.Name = pointer.Name
					}
				}
			}
			seen[a.BackendDOMNodeID] = true
			nodes = append(nodes, n)
		}
		supplemented := 0
		for id, l := range layout {
			if !l.pointer || seen[id] {
				continue
			}
			if supplemented >= 400 {
				truncated = true
				break
			}
			if n, ok := c.pointerNode(ctx, p.TargetID, frame, id); ok {
				nodes = append(nodes, n)
				supplemented++
			}
		}
	}
	frameResults = append(frameResults, c.unavailableFrames(p, frames)...)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].InViewport && !nodes[j].InViewport })
	if len(nodes) > max {
		nodes = nodes[:max]
		truncated = true
	}
	info, err := p.Context(ctx).Info()
	if err != nil {
		return nil, err
	}
	result := map[string]any{"session": c.id, "tab": c.tabName(p.TargetID), "url": truncate(info.URL, 8192), "title": truncate(info.Title, 512), "active_tab": c.activeTab(), "snapshot_id": "s_" + randID(), "frames": frameResults, "node_count": len(nodes), "truncated": truncated}
	// Bounded serialized JSON, not an invalid byte slice through a UTF-8 token.
	for {
		result["node_count"] = len(nodes)
		result["truncated"] = truncated
		if mode == "text" {
			lines := []string{}
			for _, n := range nodes {
				line := n.Role + " " + n.Name
				if n.Ref != "" {
					line = "[" + n.Ref + "] " + line
				}
				lines = append(lines, line)
			}
			result["text"] = strings.Join(lines, "\n")
		} else {
			result["nodes"] = nodes
		}
		raw, _ := json.Marshal(result)
		if len(raw) <= bytes {
			return result, nil
		}
		truncated = true
		if len(nodes) == 0 {
			return nil, errors.New("artifact_too_large")
		}
		nodes = nodes[:len(nodes)-1]
	}
}

var atomicID atomic.Uint64

func randID() string { return strconv.FormatInt(int64(atomicID.Add(1)), 36) }

// Rod's cached helper object can belong to the pre-navigation document.
// Observe readiness in a current isolated context instead of reusing it.
func (c *controlSession) waitReady(ctx context.Context, p *rod.Page) error {
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
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
			var out struct {
				Result proto.RuntimeRemoteObject `json:"result"`
			}
			if c.call(ctx, f.sid, "Runtime.evaluate", map[string]any{"expression": "({ready:document.readyState!=='loading'&&typeof window.__mycoDescribe==='function',url:location.href})", "contextId": f.context, "returnByValue": true}, &out) == nil && out.Result.Value.Get("ready").Bool() {
				info, err := p.Context(ctx).Info()
				if err == nil && info.URL == out.Result.Value.Get("url").Str() {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func (c *controlSession) unavailableFrames(p *rod.Page, ready []controlFrame) []map[string]any {
	c.browser.mu.Lock()
	root := proto.PageFrameID("")
	if installed := c.browser.installs[p.TargetID]; installed != nil {
		root = installed.rootFrame
	}
	all := map[proto.PageFrameID]frameInfo{}
	for id, f := range c.browser.frames {
		all[id] = f
	}
	c.browser.mu.Unlock()
	known := map[proto.PageFrameID]bool{}
	for _, f := range ready {
		known[f.id] = true
	}
	out := []map[string]any{}
	for id := range all {
		if known[id] {
			continue
		}
		top := id
		for depth := 0; depth < 32 && all[top].Parent != ""; depth++ {
			top = all[top].Parent
		}
		if top == root {
			out = append(out, map[string]any{"id": c.frameName(id), "status": "unavailable"})
		}
	}
	return out
}
