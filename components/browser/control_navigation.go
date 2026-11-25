package main

import (
	"encoding/json"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"net/url"
	"strings"
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
	case "Page.frameRequestedNavigation", "Page.frameStartedNavigating":
		var frameID proto.PageFrameID
		full := true
		if msg.Method == "Page.frameRequestedNavigation" {
			var event proto.PageFrameRequestedNavigation
			msg.Load(&event)
			frameID = event.FrameID
			s.mu.Lock()
			current := s.frames[frameID].URL
			s.mu.Unlock()
			trimHash := func(raw string) string {
				u, e := url.Parse(raw)
				if e != nil {
					return raw
				}
				u.Fragment = ""
				u.RawFragment = ""
				return u.String()
			}
			full = trimHash(current) != trimHash(event.URL)
		} else {
			var event frameStartedNavigation
			msg.Load(&event)
			frameID = event.FrameID
			full = event.NavigationType != "sameDocument"
		}
		s.mu.Lock()
		top = s.topNavigationLocked(msg.SessionID, frameID)
		s.mu.Unlock()
		if top && full {
			c.mu.Lock()
			if c.pending && c.navigationTracked[tid] && time.Now().Before(c.navigationUntil) {
				c.navigationLoads[tid] = true
				c.navigationPending = true
				c.signalEffectsLocked()
			}
			c.mu.Unlock()
		}
		return
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
		info := s.frames[frame]
		info.URL = raw
		s.frames[frame] = info
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
		var event proto.PageWindowOpen
		msg.Load(&event)
		c.mu.Lock()
		if c.pending && tid == c.navigationTab && time.Now().Before(c.navigationUntil) {
			c.pendingPopupURL = event.URL
		}
		c.mu.Unlock()
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
		if msg.Method == "Page.frameNavigated" {
			c.mu.Lock()
			if c.navigationTracked[tid] {
				delete(c.navigationLoads, tid)
				c.navigationPending = len(c.navigationLoads) > 0
				c.signalEffectsLocked()
			}
			c.mu.Unlock()
		}
	}
}

func (c *controlSession) signalEffectsLocked() { close(c.wake); c.wake = make(chan struct{}) }
func (c *controlSession) captureNavigation(tab proto.TargetTargetID, raw string, popup bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || tab != c.navigationTab || !time.Now().Before(c.navigationUntil) && !c.navigationPending {
		return
	}
	c.captureNavigationLocked(tab, raw, popup)
}
func (c *controlSession) captureNavigationLocked(tab proto.TargetTargetID, raw string, popup bool) {
	if c.closed || tab != c.navigationTab {
		return
	}
	value := recordedControlURL(raw)
	if c.pending {
		if value != "" {
			c.pendingURL = raw
		}
		c.pendingPopup = c.pendingPopup || popup
		c.signalEffectsLocked()
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
	if value == "" {
		return
	}
	if c.navigationNode != "" {
		for i := range c.steps {
			if c.steps[i].ID == c.navigationNode {
				c.steps[i].Value = value
				return
			}
		}
	}
	c.reservedWait = false
	n := node{ID: "n_" + randID(), Type: "wait", Wait: "url", Value: value}
	if c.budgetLocked(n) == nil {
		c.steps = append(c.steps, n)
		c.navigationNode = n.ID
	}
}

const navigationWaitValueLimit = 8192 // Bytes of the JSON string, including quotes.
func reservedNavigationValue() string { return strings.Repeat("x", navigationWaitValueLimit-2) }
func recordedControlURL(raw string) string {
	if checkedURL(raw) != nil {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if strings.Contains(parsed.Fragment, "${") {
		parsed.Fragment = ""
		parsed.RawFragment = ""
	}
	value := stableURL(parsed.String())
	if strings.HasPrefix(value, "#") {
		value = "#" + parsed.EscapedFragment()
	}
	if strings.Contains(value, "${") {
		parsed.Fragment = ""
		parsed.RawFragment = ""
		value = stableURL(parsed.String())
	}
	if strings.Contains(value, "${") {
		return ""
	}
	// A URL wait is a substring assertion. Bound the literal prefix by its
	// actual encoded size so escaping cannot consume 48 KiB of draft reserve.
	encoded, _ := json.Marshal(value)
	if len(encoded) <= navigationWaitValueLimit {
		return value
	}
	runes := []rune(value)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		encoded, _ = json.Marshal(string(runes[:mid]))
		if len(encoded) <= navigationWaitValueLimit {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return string(runes[:low])
}
