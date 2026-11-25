package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

type controlLayout struct {
	visible   bool
	pointer   bool
	sensitive bool
}

// Computed cursor and click listeners supplement the accessibility tree for
// custom controls without a role. Raw DOM snapshot strings never leave here.
func (c *controlSession) layout(ctx context.Context, frame controlFrame, snapshot *proto.DOMSnapshotCaptureSnapshotResult) map[proto.DOMBackendNodeID]controlLayout {
	out := map[proto.DOMBackendNodeID]controlLayout{}
	if snapshot == nil {
		return out
	}
	var view struct {
		Result proto.RuntimeRemoteObject `json:"result"`
	}
	if c.call(ctx, frame.sid, "Runtime.evaluate", map[string]any{"expression": "({w:innerWidth,h:innerHeight,x:scrollX,y:scrollY})", "contextId": frame.context, "returnByValue": true}, &view) != nil {
		return out
	}
	width, height := view.Result.Value.Get("w").Num(), view.Result.Value.Get("h").Num()
	x, y := view.Result.Value.Get("x").Num(), view.Result.Value.Get("y").Num()
	for _, doc := range snapshot.Documents {
		if doc == nil || doc.Nodes == nil || doc.Layout == nil || int(doc.FrameID) >= len(snapshot.Strings) || snapshot.Strings[doc.FrameID] != string(frame.id) {
			continue
		}
		for index, id := range doc.Nodes.BackendNodeID {
			if index >= len(doc.Nodes.Attributes) {
				continue
			}
			attrs := map[string]string{}
			pairs := doc.Nodes.Attributes[index]
			for j := 0; j+1 < len(pairs); j += 2 {
				a, b := int(pairs[j]), int(pairs[j+1])
				if a >= 0 && b >= 0 && a < len(snapshot.Strings) && b < len(snapshot.Strings) {
					attrs[strings.ToLower(snapshot.Strings[a])] = snapshot.Strings[b]
				}
			}
			if sensitiveAttributes(attrs) {
				out[id] = controlLayout{sensitive: true}
			}
		}
		clickable := map[int]bool{}
		if doc.Nodes.IsClickable != nil {
			for _, index := range doc.Nodes.IsClickable.Index {
				clickable[index] = true
			}
		}
		for i, index := range doc.Layout.NodeIndex {
			if index < 0 || index >= len(doc.Nodes.BackendNodeID) || i >= len(doc.Layout.Bounds) {
				continue
			}
			box := doc.Layout.Bounds[i]
			if len(box) < 4 {
				continue
			}
			pointer := clickable[index]
			if i < len(doc.Layout.Styles) && len(doc.Layout.Styles[i]) > 0 {
				style := doc.Layout.Styles[i][0]
				if int(style) >= 0 && int(style) < len(snapshot.Strings) && snapshot.Strings[style] == "pointer" {
					pointer = true
				}
			}
			id := doc.Nodes.BackendNodeID[index]
			meta := out[id]
			meta.visible = box[2] > 0 && box[3] > 0 && box[0]+box[2] > x && box[1]+box[3] > y && box[0] < x+width && box[1] < y+height
			meta.pointer = pointer
			out[id] = meta
		}
	}
	return out
}
func (c *controlSession) pointerNode(ctx context.Context, pid proto.TargetTargetID, frame controlFrame, id proto.DOMBackendNodeID) (controlNode, bool) {
	r := controlRef{tab: pid, frame: frame.id, sid: frame.sid, epoch: frame.epoch, backend: id}
	info, err := c.inspect(ctx, r)
	if err != nil || info.NodeType != 1 || !info.Visible {
		return controlNode{}, false
	}
	name := "[redacted]"
	if !info.Sensitive {
		object, err := c.resolve(ctx, r)
		if err != nil {
			return controlNode{}, false
		}
		var text struct {
			Text string `json:"text"`
		}
		err = c.function(ctx, r.sid, object, "extract", map[string]any{"what": "text"}, &text)
		_ = c.call(ctx, r.sid, "Runtime.releaseObject", map[string]any{"objectId": object}, nil)
		if err != nil {
			return controlNode{}, false
		}
		name = truncate(text.Text, 512)
	}
	r.generation = info.Generation
	r.bound = true
	return controlNode{ID: frame.name + "p" + strconv.Itoa(int(id)), Ref: c.ref(r), Frame: frame.name, Role: "generic", Name: name, Interactive: true, Disabled: info.Disabled, Redacted: info.Sensitive, Rect: info.Rect, InViewport: info.InViewport}, true
}
