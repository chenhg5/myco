package main

import (
	"encoding/json"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"time"
)

func (s *session) controlEvent(msg *rod.Message) {
	s.mu.Lock()
	c := s.control
	tid := s.targets[msg.SessionID]
	s.mu.Unlock()
	if c == nil {
		return
	}
	var frame proto.PageFrameID
	var raw string
	popup := false
	top := false
	switch msg.Method {
	case "Runtime.bindingCalled":
		var event proto.RuntimeBindingCalled
		msg.Load(&event)
		if event.Name != "__mycoBrowserEvent" {
			return
		}
		s.mu.Lock()
		_, owned := s.contexts[string(msg.SessionID)][event.ExecutionContextID]
		s.mu.Unlock()
		if !owned {
			return
		}
		var signal struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal([]byte(event.Payload), &signal) == nil && signal.Type == "control-barrier" {
			c.mu.Lock()
			if done := c.barriers[signal.ID]; done != nil {
				close(done)
				delete(c.barriers, signal.ID)
			}
			c.mu.Unlock()
		}
		return
	case "Page.frameNavigated":
		var event proto.PageFrameNavigated
		msg.Load(&event)
		if event.Frame == nil {
			return
		}
		frame = event.Frame.ID
		raw = event.Frame.URL
		s.mu.Lock()
		installed := s.installs[tid]
		top = installed != nil && installed.kind == "page" && event.Frame.ParentID == ""
		s.mu.Unlock()
	case "Page.navigatedWithinDocument":
		var event proto.PageNavigatedWithinDocument
		msg.Load(&event)
		frame, raw = event.FrameID, event.URL
		s.mu.Lock()
		top = s.topNavigationLocked(msg.SessionID, frame)
		s.mu.Unlock()
	case "Page.frameDetached":
		var event proto.PageFrameDetached
		msg.Load(&event)
		frame = event.FrameID
	case "Target.detachedFromTarget":
		var event proto.TargetDetachedFromTarget
		msg.Load(&event)
		c.mu.Lock()
		for _, r := range c.refs {
			if r.sid == event.SessionID {
				c.epochs[r.frame]++
			}
		}
		c.mu.Unlock()
		return
	case "Page.windowOpen":
		popup = true
		top = true
	default:
		return
	}
	if frame != "" {
		s.mu.Lock()
		descendants := []proto.PageFrameID{frame}
		for id := range s.frames {
			ancestor := id
			for i := 0; i < 32; i++ {
				if ancestor == frame {
					descendants = append(descendants, id)
					break
				}
				ancestor = s.frames[ancestor].Parent
				if ancestor == "" {
					break
				}
			}
		}
		s.mu.Unlock()
		c.mu.Lock()
		for _, id := range descendants {
			c.epochs[id]++
		}
		for id, r := range c.refs {
			if c.epochs[r.frame] != r.epoch {
				delete(c.refs, id)
				delete(c.refKeys, r)
			}
		}
		c.mu.Unlock()
	}
	if top {
		if !popup && checkedURL(raw) == nil && controlHostAllowed(c.hosts, raw) {
			c.mu.Lock()
			c.allowedURLs[tid] = raw
			c.mu.Unlock()
		}
		c.enforceHostsSoon()
		c.captureNavigation(tid, raw, popup)
	}
}
func (c *controlSession) captureNavigation(tab proto.TargetTargetID, raw string, popup bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !time.Now().Before(c.navigationUntil) || tab != c.navigationTab {
		return
	}
	if c.pending {
		if raw != "" && checkedURL(raw) == nil {
			c.pendingURL = raw
		}
		c.pendingPopup = c.pendingPopup || popup
		return
	}
	if popup {
		if c.popupNode != "" {
			return
		}
		c.reservedPopup = false
		n := node{ID: "n_" + randID(), Type: "tab", Tab: "popup"}
		if c.budgetLocked(n) == nil {
			c.steps = append(c.steps, n)
			c.popupNode = n.ID
		}
		return
	}
	if checkedURL(raw) != nil {
		return
	}
	if c.navigationNode != "" {
		for i := range c.steps {
			if c.steps[i].ID == c.navigationNode {
				c.steps[i].Value = stableURL(raw)
				return
			}
		}
	}
	c.reservedWait = false
	n := node{ID: "n_" + randID(), Type: "wait", Wait: "url", Value: stableURL(raw)}
	if c.budgetLocked(n) == nil {
		c.steps = append(c.steps, n)
		c.navigationNode = n.ID
	}
}
