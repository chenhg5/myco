package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sort"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type screenshotPrivacyPlan struct {
	Masks     []controlRect
	Documents map[proto.PageFrameID]string
	Epochs    map[proto.PageFrameID]uint64
}

// Reasons are fixed categories. They must never include page content or URLs.
type screenshotPrivacyError string

func (e screenshotPrivacyError) Error() string { return "sensitive_snapshot" }

// Anchor the root in the live frame tree and discover child owners in the
// current DOM. OOPIFs may be omitted from the root tree, while historical
// advertising-frame events may still describe already detached documents.
func (c *controlSession) screenshotPrivacy(ctx context.Context, p *rod.Page) (screenshotPrivacyPlan, error) {
	plan := screenshotPrivacyPlan{Documents: map[proto.PageFrameID]string{}, Epochs: map[proto.PageFrameID]uint64{}}
	deny := func(reason screenshotPrivacyError) (screenshotPrivacyPlan, error) { return plan, reason }
	var tree proto.PageGetFrameTreeResult
	if c.call(ctx, c.browser.primarySession(p.TargetID), "Page.getFrameTree", nil, &tree) != nil || tree.FrameTree == nil || tree.FrameTree.Frame == nil {
		return deny("privacy_unavailable")
	}
	root := tree.FrameTree.Frame.ID
	parents := map[proto.PageFrameID]proto.PageFrameID{root: ""}
	plan.Documents[root] = tree.FrameTree.Frame.URL
	ready := map[proto.PageFrameID]controlFrame{}
	for _, frame := range c.framesFor(p) {
		ready[frame.id] = frame
	}
	if _, ok := ready[root]; !ok {
		return deny("privacy_unavailable")
	}
	owners := map[proto.PageFrameID]controlRef{}
	queue := []proto.PageFrameID{root}
	checked := map[proto.TargetSessionID]bool{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		frame := ready[id]
		plan.Epochs[id] = frame.epoch
		if checked[frame.sid] {
			continue
		}
		checked[frame.sid] = true
		var doc proto.DOMGetDocumentResult
		if c.call(ctx, frame.sid, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, &doc) != nil {
			if id == root {
				return deny("privacy_unavailable")
			}
			delete(ready, id)
			delete(plan.Epochs, id)
			continue
		}
		denial := screenshotPrivacyError("protected_content")
		var safe func(*proto.DOMNode, proto.PageFrameID) bool
		safe = func(n *proto.DOMNode, current proto.PageFrameID) bool {
			if n == nil {
				return true
			}
			attrs := map[string]string{"tag": n.NodeName}
			for i := 0; i+1 < len(n.Attributes); i += 2 {
				attrs[strings.ToLower(n.Attributes[i])] = strings.ToLower(n.Attributes[i+1])
			}
			if attrs["id"] == "__myco_recording_panel__" {
				return true
			}
			if sensitiveAttributes(attrs) {
				return false
			}
			child := current
			if n.NodeName == "IFRAME" || n.NodeName == "FRAME" {
				child = n.FrameID
				if child == "" {
					child = proto.PageFrameID(fmt.Sprint("pending-owner-", n.BackendNodeID))
				}
				parents[child] = current
				plan.Documents[child] = ""
				parent, ok := ready[current]
				if !ok {
					denial = "privacy_unavailable"
					return false
				}
				owners[child] = controlRef{sid: frame.sid, frame: current, epoch: parent.epoch, tab: p.TargetID, backend: n.BackendNodeID}
				if f, readable := ready[child]; !readable {
					return true
				} else {
					plan.Epochs[child] = f.epoch
					queue = append(queue, child)
				}
			}
			for _, nested := range n.Children {
				if !safe(nested, current) {
					return false
				}
			}
			for _, nested := range n.ShadowRoots {
				if !safe(nested, current) {
					return false
				}
			}
			return safe(n.ContentDocument, child)
		}
		if !safe(doc.Root, id) {
			return deny(denial)
		}
	}
	ids := []proto.PageFrameID{}
	for id := range parents {
		if _, ok := ready[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	masked := map[proto.PageFrameID]bool{}
	for _, id := range ids {
		outer := id
		for depth := 0; depth < 32; depth++ {
			parent := parents[outer]
			if parent == "" {
				return deny("privacy_unavailable")
			}
			if _, ok := ready[parent]; ok {
				break
			}
			outer = parent
		}
		if masked[outer] {
			continue
		}
		masked[outer] = true
		rect, err := c.unreadableFrameRect(ctx, p, outer, parents, ready, owners)
		if err != nil {
			return deny("frame_unavailable")
		}
		if rect.Width > 0 && rect.Height > 0 {
			plan.Masks = append(plan.Masks, rect)
		}
	}
	return plan, nil
}

func (c *controlSession) unreadableFrameRect(ctx context.Context, p *rod.Page, id proto.PageFrameID, parents map[proto.PageFrameID]proto.PageFrameID, ready map[proto.PageFrameID]controlFrame, owners map[proto.PageFrameID]controlRef) (controlRect, error) {
	ownerInfo := func(child proto.PageFrameID) (controlDOMInfo, error) {
		if owner, ok := owners[child]; ok {
			return c.inspect(ctx, owner)
		}
		parent, ok := ready[parents[child]]
		if !ok {
			return controlDOMInfo{}, errors.New("sensitive_snapshot")
		}
		var owner struct {
			Backend proto.DOMBackendNodeID `json:"backendNodeId"`
		}
		if err := c.call(ctx, parent.sid, "DOM.getFrameOwner", map[string]any{"frameId": child}, &owner); err != nil {
			return controlDOMInfo{}, err
		}
		return c.inspect(ctx, controlRef{sid: parent.sid, frame: parent.id, epoch: parent.epoch, tab: p.TargetID, backend: owner.Backend})
	}
	info, err := ownerInfo(id)
	if err != nil || !info.Connected || info.Rect == nil {
		return controlRect{}, errors.New("sensitive_snapshot")
	}
	if !info.Visible {
		return controlRect{}, nil
	}
	rect := *info.Rect
	frame := parents[id]
	for depth := 0; parents[frame] != "" && depth < 32; depth++ {
		info, err = ownerInfo(frame)
		if err != nil || info.Rect == nil || !info.Connected {
			return controlRect{}, errors.New("sensitive_snapshot")
		}
		if !info.Visible {
			return controlRect{}, nil
		}
		if !info.AxisAligned {
			rect = *info.Rect
		} else {
			rect.X = info.Rect.X + (rect.X+info.BorderX)*info.ScaleX
			rect.Y = info.Rect.Y + (rect.Y+info.BorderY)*info.ScaleY
			rect.Width *= info.ScaleX
			rect.Height *= info.ScaleY
		}
		frame = parents[frame]
	}
	return rect, nil
}

func sameScreenshotPrivacy(a, b screenshotPrivacyPlan) bool {
	// A same-document fragment navigation can happen while pixels are captured.
	// It does not replace the document; frame epochs and the DOM privacy scan
	// below still detect changes that could expose sensitive content.
	normalize := func(plan screenshotPrivacyPlan) screenshotPrivacyPlan {
		documents := make(map[proto.PageFrameID]string, len(plan.Documents))
		for id, document := range plan.Documents {
			documents[id], _, _ = strings.Cut(document, "#")
		}
		plan.Documents = documents
		return plan
	}
	a, b = normalize(a), normalize(b)
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// Redact in memory before any artifact is persisted or returned. Coordinates
// are CSS pixels in the top viewport; the PNG may have a different pixel scale.
func maskScreenshot(data []byte, masks []controlRect, viewX, viewY float64, clip *proto.PageViewport) ([]byte, error) {
	if len(masks) == 0 {
		return data, nil
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
	sx, sy := float64(bounds.Dx())/clip.Width, float64(bounds.Dy())/clip.Height
	for _, r := range masks {
		x, y := (r.X+viewX-clip.X)*sx, (r.Y+viewY-clip.Y)*sy
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			return nil, errors.New("sensitive_snapshot")
		}
		box := image.Rect(int(math.Floor(x))-1, int(math.Floor(y))-1, int(math.Ceil(x+r.Width*sx))+1, int(math.Ceil(y+r.Height*sy))+1).Intersect(bounds)
		draw.Draw(dst, box, image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	var out bytes.Buffer
	err = png.Encode(&out, dst)
	return out.Bytes(), err
}
