package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

//go:embed recorder.js
var recorderSource string

type engineInternal struct{}

const recorderWorld = "myco-browser-collector"

func (s *session) trace(method string, sid proto.TargetSessionID, done bool) {
	if s.e != nil && s.e.trace != nil {
		s.e.trace(string(sid), method, done)
	}
}
func (s *session) call(sid proto.TargetSessionID, method string, args any, out any) error {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, engineInternal{}, true)
	raw, err := s.browser.Call(ctx, string(sid), method, args)
	if err != nil {
		return err
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}
func (s *session) attach() error {
	events := s.browser.Event()
	go func() {
		for msg := range events {
			s.event(msg)
		}
	}()
	dir, err := s.e.downloadDir(s.id)
	if err != nil {
		return err
	}
	_, err = s.browser.Call(s.ctx, "", "Browser.setDownloadBehavior", map[string]any{"behavior": "allowAndName", "downloadPath": dir, "eventsEnabled": true})
	if err != nil {
		return err
	}
	if err = s.call("", "Target.setDiscoverTargets", map[string]any{"discover": true}, nil); err != nil {
		return err
	}
	return s.call("", "Target.setAutoAttach", autoAttach(), nil)
}
func autoAttach() map[string]any {
	return map[string]any{"autoAttach": true, "flatten": true, "waitForDebuggerOnStart": true, "filter": []map[string]any{{"type": "page"}, {"type": "iframe"}, {"exclude": true}}}
}
func (s *session) installTarget(evt proto.TargetAttachedToTarget) (err error) {
	sid, info := evt.SessionID, evt.TargetInfo
	if info.Type != "page" && info.Type != "iframe" {
		return nil
	}
	s.mu.Lock()
	s.targets[sid] = info.TargetID
	if previous := s.installs[info.TargetID]; previous != nil {
		s.mu.Unlock()
		// Every auto-attachment may wait for its own debugger. Resume a
		// secondary session too, once the primary collector is configured.
		if previous.configured != nil {
			select {
			case <-previous.configured:
			case <-s.ctx.Done():
				return s.ctx.Err()
			}
		}
		return s.call(sid, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil)
	}
	installed := &targetInstall{kind: info.Type, sid: sid, ready: make(chan struct{}), configured: make(chan struct{})}
	s.installs[info.TargetID] = installed
	var configuredOnce sync.Once
	configured := func() { configuredOnce.Do(func() { close(installed.configured) }) }
	defer configured()
	// A secondary Rod attachment must not resume a target while the
	// primary collector is still installing its bindings and new-doc script.
	defer func() { _ = s.call(sid, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil) }()
	if s.observed != nil {
		close(s.observed)
	}
	s.observed = make(chan struct{})
	s.mu.Unlock()
	defer func() { installed.err = err; close(installed.ready) }()
	s.mu.Lock()
	control := s.control
	s.mu.Unlock()
	if control != nil && info.Type == "page" && info.OpenerID != "" {
		// A headed cross-site popup can block even Page.enable until its
		// debugger pause is resumed. Agent control records explicit actions,
		// so it can resume first and wait for its isolated world before use.
		// Human recording retains installation-before-resume ordering.
		if err = s.call(sid, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil); err != nil {
			return err
		}
	}
	for _, domain := range []string{"Page", "Runtime", "DOM", "Overlay", "Network"} {
		if err = s.call(sid, domain+".enable", map[string]any{}, nil); err != nil {
			return err
		}
	}
	if err = s.call(sid, "Runtime.addBinding", map[string]any{"name": "__mycoBrowserEvent", "executionContextName": recorderWorld}, nil); err != nil {
		return err
	}
	if err = s.call(sid, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": s.collectorScript(), "worldName": recorderWorld, "runImmediately": true}, nil); err != nil {
		return err
	}
	// Iframe descendants must run immediately: pausing them can deadlock
	// verification widgets before their attachment arrives. Preloads and
	// existing-context injection still install the collector. Pages/popups
	// retain their paused bootstrap so their collector is installed first.
	nested := autoAttach()
	if info.Type == "iframe" {
		nested["waitForDebuggerOnStart"] = false
	}
	if err = s.call(sid, "Target.setAutoAttach", nested, nil); err != nil {
		return err
	}
	// A renderer-created popup is paused at window.open. Rod's attachment
	// (and Runtime.evaluate) can wait for that renderer to start. Resume
	// after the binding/preload are installed, before requesting either.
	configured()
	if err = s.call(sid, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil); err != nil {
		return err
	}
	var tree struct {
		FrameTree *structFrameTree `json:"frameTree"`
	}
	if s.call(sid, "Page.getFrameTree", map[string]any{}, &tree) == nil {
		if info.Type == "page" && tree.FrameTree != nil {
			s.mu.Lock()
			installed.rootFrame = tree.FrameTree.Frame.ID
			s.mu.Unlock()
		}
		var create func(*structFrameTree)
		create = func(frame *structFrameTree) {
			if frame == nil {
				return
			}
			var world struct {
				ID proto.RuntimeExecutionContextID `json:"executionContextId"`
			}
			if s.call(sid, "Page.createIsolatedWorld", map[string]any{"frameId": frame.Frame.ID, "worldName": recorderWorld}, &world) == nil {
				s.mu.Lock()
				if s.contexts[string(sid)] == nil {
					s.contexts[string(sid)] = map[proto.RuntimeExecutionContextID]proto.PageFrameID{}
				}
				s.contexts[string(sid)][world.ID] = frame.Frame.ID
				s.mu.Unlock()
				s.setContextRecording(sid, world.ID, false)
			}
			for _, child := range frame.Children {
				create(child)
			}
		}
		create(tree.FrameTree)
	}
	if info.Type == "page" {
		p, err := s.browser.PageFromTarget(info.TargetID)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.pages[info.TargetID] = p
		if s.current == "" {
			s.current = info.TargetID
		}
		rec, control := s.recorder, s.control
		s.mu.Unlock()
		if control != nil {
			control.tabName(info.TargetID)
			if info.OpenerID != "" {
				s.mu.Lock()
				s.current = info.TargetID
				s.mu.Unlock()
			}
			control.enforceHostsSoon()
			go s.updatePanel("")
		}
		if rec != nil {
			rec.tabIndex(info.TargetID)
			s.mu.Lock()
			s.current = info.TargetID
			s.mu.Unlock()
			if info.OpenerID == "" {
				rec.enqueue(node{Type: "tab", Tab: "new"}, p)
			}
		}
	}

	return nil
}

func (s *session) event(msg *rod.Message) {
	if strings.HasPrefix(msg.Method, "Page.") || strings.HasPrefix(msg.Method, "Runtime.") {
		s.mu.Lock()
		primary := s.installs[s.targets[msg.SessionID]]
		s.mu.Unlock()
		if primary != nil && primary.sid != msg.SessionID {
			return
		}
	}
	s.controlEvent(msg)
	switch msg.Method {
	case "Target.attachedToTarget":
		var evt proto.TargetAttachedToTarget
		msg.Load(&evt)
		if evt.TargetInfo != nil && evt.TargetInfo.Type == "page" {
			s.mu.Lock()
			if s.seenTargets == nil {
				s.seenTargets = map[proto.TargetTargetID]bool{}
			}
			first := !s.seenTargets[evt.TargetInfo.TargetID]
			s.seenTargets[evt.TargetInfo.TargetID] = true
			s.mu.Unlock()
			if first {
				s.observe(observation{Kind: "target", Target: evt.TargetInfo.TargetID, Session: evt.SessionID, Opener: evt.TargetInfo.OpenerID, OpenerFrame: evt.TargetInfo.OpenerFrameID})
			}
		}
		go func() {
			if s.installTarget(evt) != nil {
				s.mu.Lock()
				rec := s.recorder
				s.mu.Unlock()
				if rec != nil {
					rec.mu.Lock()
					rec.err = "capture_failed"
					rec.mu.Unlock()
					rec.finish()
				}
			}
		}()
	case "Target.detachedFromTarget":
		var evt proto.TargetDetachedFromTarget
		msg.Load(&evt)
		s.mu.Lock()
		tid := s.targets[evt.SessionID]
		delete(s.targets, evt.SessionID)
		if current := s.installs[tid]; current != nil && current.sid == evt.SessionID {
			delete(s.installs, tid)
		}
		s.mu.Unlock()
	case "Target.targetDestroyed":
		var evt proto.TargetTargetDestroyed
		msg.Load(&evt)
		s.mu.Lock()
		delete(s.pages, evt.TargetID)
		if s.current == evt.TargetID {
			s.current = ""
			for id := range s.pages {
				s.current = id
				break
			}
		}
		s.mu.Unlock()
	case "Runtime.executionContextCreated":
		var evt proto.RuntimeExecutionContextCreated
		msg.Load(&evt)
		if evt.Context.Name != recorderWorld {
			return
		}
		frame := proto.PageFrameID(evt.Context.AuxData["frameId"].Str())
		s.mu.Lock()
		if s.contexts[string(msg.SessionID)] == nil {
			s.contexts[string(msg.SessionID)] = map[proto.RuntimeExecutionContextID]proto.PageFrameID{}
		}
		s.contexts[string(msg.SessionID)][evt.Context.ID] = frame
		active := s.recorder != nil
		s.mu.Unlock()
		go s.setContextRecording(msg.SessionID, evt.Context.ID, active)
	case "Runtime.executionContextsCleared":
		s.mu.Lock()
		delete(s.contexts, string(msg.SessionID))
		s.mu.Unlock()
	case "Runtime.bindingCalled":
		var evt proto.RuntimeBindingCalled
		msg.Load(&evt)
		if evt.Name != "__mycoBrowserEvent" || len(evt.Payload) > 32<<10 {
			return
		}
		s.mu.Lock()
		rec := s.recorder
		frame := s.contexts[string(msg.SessionID)][evt.ExecutionContextID]
		tid := s.targets[msg.SessionID]
		p := s.pages[tid]
		if p == nil {
			p = s.pages[s.current]
		}
		s.mu.Unlock()
		var command struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(evt.Payload), &command) == nil && command.Type == "panel" {
			go s.panelCommand(msg.SessionID, frame, evt.Payload)
			return
		}
		if rec == nil {
			return
		}
		var n node
		if json.Unmarshal([]byte(evt.Payload), &n) != nil {
			return
		}
		if n.Type == "barrier" {
			rec.mu.Lock()
			if done := rec.barriers[n.ID]; done != nil {
				close(done)
				delete(rec.barriers, n.ID)
			}
			rec.mu.Unlock()
			return
		}
		if !recordable(n) {
			return
		}
		s.mu.Lock()
		if s.pages[tid] != nil && s.current != tid {
			s.current = tid
			rec.enqueue(node{Type: "tab", Tab: strconv.Itoa(rec.tabIndex(tid))}, p)
		}
		s.mu.Unlock()
		if p != nil {
			rec.noteNavigationAction(p.TargetID, n, time.Now())
		}
		rec.enqueueFrame(n, p, frame)
	case "Page.frameStartedNavigating":
		var evt frameStartedNavigation
		msg.Load(&evt)
		s.mu.Lock()
		delete(s.browserNavigations, evt.FrameID)
		if evt.NavigationType == "sameDocument" && s.recorder != nil && s.topNavigationLocked(msg.SessionID, evt.FrameID) {
			if s.browserNavigations == nil {
				s.browserNavigations = map[proto.PageFrameID]recordedBrowserNavigation{}
			}
			s.browserNavigations[evt.FrameID] = recordedBrowserNavigation{RecordID: s.recorder.id, URL: evt.URL}
		}
		s.mu.Unlock()
	case "Page.frameRequestedNavigation":
		var evt proto.PageFrameRequestedNavigation
		msg.Load(&evt)
		s.mu.Lock()
		top := s.topNavigationLocked(msg.SessionID, evt.FrameID)
		rec := s.recorder
		p := s.pages[s.targets[msg.SessionID]]
		s.mu.Unlock()
		if !top {
			return
		}
		request := recordedNavigationRequest{}
		if rec != nil && p != nil {
			request.RecordID = rec.id
			request.User = rec.recentNavigationAction(p.TargetID, time.Now())
		}
		s.mu.Lock()
		if s.rendererNavigations == nil {
			s.rendererNavigations = map[proto.PageFrameID]recordedNavigationRequest{}
		}
		s.rendererNavigations[evt.FrameID] = request
		s.mu.Unlock()
	case "Page.frameNavigated":
		var evt proto.PageFrameNavigated
		msg.Load(&evt)
		if evt.Frame == nil {
			return
		}
		s.mu.Lock()
		rec := s.recorder
		previous := s.frames[evt.Frame.ID]
		s.frames[evt.Frame.ID] = frameInfo{Parent: evt.Frame.ParentID, URL: evt.Frame.URL, Selector: previous.Selector}
		installed := s.installs[s.targets[msg.SessionID]]
		if installed != nil && installed.kind == "page" && evt.Frame.ParentID == "" {
			installed.rootFrame = evt.Frame.ID
		}
		top := evt.Frame.ParentID == "" && s.topNavigationLocked(msg.SessionID, evt.Frame.ID)
		request, renderer := s.rendererNavigations[evt.Frame.ID]
		delete(s.rendererNavigations, evt.Frame.ID)
		delete(s.browserNavigations, evt.Frame.ID)
		p := s.pages[s.targets[msg.SessionID]]
		s.mu.Unlock()
		if !top || rec == nil || p == nil || checkedURL(evt.Frame.URL) != nil {
			return
		}
		if renderer {
			if request.RecordID == rec.id && request.User {
				rec.enqueue(node{Type: "wait", Wait: "url", Value: stableURL(evt.Frame.URL)}, p)
			}
		} else {
			// Browser-initiated navigation (including address-bar entry) has no
			// renderer request. Preserve it as an explicit navigation step.
			rec.enqueue(node{Type: "navigate", URL: navigationURL(evt.Frame.URL)}, p)
		}
	case "Page.navigatedWithinDocument":
		var evt proto.PageNavigatedWithinDocument
		msg.Load(&evt)
		s.mu.Lock()
		rec := s.recorder
		top := s.topNavigationLocked(msg.SessionID, evt.FrameID)
		p := s.pages[s.targets[msg.SessionID]]
		request, renderer := s.rendererNavigations[evt.FrameID]
		delete(s.rendererNavigations, evt.FrameID)
		browserStart := s.browserNavigations[evt.FrameID]
		delete(s.browserNavigations, evt.FrameID)
		s.mu.Unlock()
		if !top || rec == nil || p == nil || checkedURL(evt.URL) != nil {
			return
		}
		if renderer {
			if request.RecordID == rec.id && request.User {
				rec.enqueue(node{Type: "wait", Wait: "url", Value: stableURL(evt.URL)}, p)
			}
		} else if browserStart.RecordID == rec.id && browserStart.URL == evt.URL {
			rec.enqueue(node{Type: "navigate", URL: navigationURL(evt.URL)}, p)
		} else if rec.recentNavigationAction(p.TargetID, time.Now()) {
			rec.enqueue(node{Type: "wait", Wait: "url", Value: stableURL(evt.URL)}, p)
		}
	case "Page.windowOpen":
		var evt proto.PageWindowOpen
		msg.Load(&evt)
		s.mu.Lock()
		rec := s.recorder
		s.mu.Unlock()
		if rec != nil {
			rec.enqueue(node{Type: "tab", Tab: "popup"}, nil)
		}
	case "Page.javascriptDialogOpening":
		var evt proto.PageJavascriptDialogOpening
		msg.Load(&evt)
		s.mu.Lock()
		if s.dialogs == nil {
			s.dialogs = map[proto.TargetSessionID]string{}
		}
		s.dialogs[msg.SessionID] = string(evt.Type)
		target := s.targets[msg.SessionID]
		s.mu.Unlock()
		s.observe(observation{Kind: "dialog", Target: target, Session: msg.SessionID})
	case "Page.javascriptDialogClosed":
		var evt proto.PageJavascriptDialogClosed
		msg.Load(&evt)
		s.mu.Lock()
		rec := s.recorder
		kind := s.dialogs[msg.SessionID]
		delete(s.dialogs, msg.SessionID)
		s.mu.Unlock()
		if rec != nil {
			choice := "dismiss"
			if evt.Result {
				choice = "accept"
			}
			n := node{Type: "dialog", Dialog: choice}
			if kind == "prompt" && evt.Result && len(evt.UserInput) > 0 {
				n.Value = "${dialog_input}"
				n.Sensitive = true
			}
			rec.enqueue(n, nil)
		}
	case "Overlay.inspectNodeRequested":
		var evt proto.OverlayInspectNodeRequested
		msg.Load(&evt)
		go s.pickNode(msg.SessionID, evt.BackendNodeID)
	case "Browser.downloadWillBegin":
		var evt proto.BrowserDownloadWillBegin
		msg.Load(&evt)
		s.mu.Lock()
		rec := s.recorder
		if s.downloadNames == nil {
			s.downloadNames = map[string]string{}
		}
		s.downloadNames[evt.GUID] = evt.SuggestedFilename
		s.mu.Unlock()
		s.observe(observation{Kind: "download", ID: evt.GUID})
		if rec != nil {
			rec.enqueue(node{Type: "wait", Wait: "download"}, nil)
		}
	case "Browser.downloadProgress":
		var evt proto.BrowserDownloadProgress
		msg.Load(&evt)
		if evt.State == proto.BrowserDownloadProgressStateCompleted || evt.State == proto.BrowserDownloadProgressStateCanceled {
			s.mu.Lock()
			name := s.downloadNames[evt.GUID]
			s.mu.Unlock()
			code := ""
			if evt.State == proto.BrowserDownloadProgressStateCanceled {
				code = "cancelled"
			}
			if code == "" {
				s.registerDownload(evt.GUID)
			}
			s.observe(observation{Kind: "download-complete", ID: evt.GUID, Filename: name, Error: code})
		}
	}
}

func (s *session) setContextRecording(sid proto.TargetSessionID, id proto.RuntimeExecutionContextID, _ bool) {
	raw, _ := json.Marshal(s.recordingPanelState(""))
	expression := s.collectorScript() + ";if(window.__mycoRecorder)window.__mycoRecorder.updatePanel(" + string(raw) + ",true)"
	_ = s.call(sid, "Runtime.evaluate", map[string]any{"expression": expression, "contextId": id}, nil)
}
func (s *session) setRecording(enabled bool) {
	s.mu.Lock()
	copy := map[proto.TargetSessionID][]proto.RuntimeExecutionContextID{}
	for sid, contexts := range s.contexts {
		for id := range contexts {
			copy[proto.TargetSessionID(sid)] = append(copy[proto.TargetSessionID(sid)], id)
		}
	}
	s.mu.Unlock()
	for sid, ids := range copy {
		for _, id := range ids {
			s.setContextRecording(sid, id, enabled)
		}
	}
}
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
func recordable(n node) bool {
	switch n.Type {
	case "click", "type", "select", "press", "scroll", "drag", "upload":
	default:
		return false
	}
	if n.Target == nil || len(n.Target.Locators) == 0 || len(n.Target.Locators) > 12 || len(n.Target.Frame) > 16 || len(n.Target.Shadow) > 16 {
		return false
	}
	if n.Sensitive {
		return n.Type == "type" && n.Value == "${password}"
	}
	// A website cannot smuggle arbitrary local upload paths through a binding.
	if n.Type == "upload" && n.Value != "${upload}" {
		return false
	}
	return len(n.Value) <= 16000 && len(n.Key) <= 160
}

var errRecordingOverflow = errors.New("recording_limit")

// Resolve into our isolated world so a site's scripts cannot replace the
// binding or forge collector globals. Only the matching document's world is used.
func (s *session) describeBackend(sid proto.TargetSessionID, id proto.DOMBackendNodeID) (target, proto.PageFrameID, error) {
	s.mu.Lock()
	worlds := map[proto.RuntimeExecutionContextID]proto.PageFrameID{}
	for contextID, frame := range s.contexts[string(sid)] {
		worlds[contextID] = frame
	}
	s.mu.Unlock()
	for contextID, frame := range worlds {
		var object struct {
			Object *proto.RuntimeRemoteObject `json:"object"`
		}
		if s.call(sid, "DOM.resolveNode", map[string]any{"backendNodeId": id, "executionContextId": contextID}, &object) != nil || object.Object == nil {
			continue
		}
		var result struct {
			Result *proto.RuntimeRemoteObject `json:"result"`
		}
		err := s.call(sid, "Runtime.callFunctionOn", map[string]any{"objectId": object.Object.ObjectID, "functionDeclaration": "function(){if(this.ownerDocument.defaultView!==window)return null;return window.__mycoDescribe(this)}", "returnByValue": true}, &result)
		_ = s.call(sid, "Runtime.releaseObject", map[string]any{"objectId": object.Object.ObjectID}, nil)
		if err != nil || result.Result == nil {
			continue
		}
		raw, _ := json.Marshal(result.Result.Value.Val())
		var out target
		if json.Unmarshal(raw, &out) == nil && len(out.Locators) > 0 {
			return out, frame, nil
		}
	}
	return target{}, "", errors.New("frame_unavailable")
}

func (s *session) primarySession(target proto.TargetTargetID) proto.TargetSessionID {
	s.mu.Lock()
	defer s.mu.Unlock()
	if installed := s.installs[target]; installed != nil {
		return installed.sid
	}
	return ""
}

func stableURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	fragment := strings.SplitN(u.Fragment, "?", 2)[0]
	if fragment != "" && !strings.Contains(fragment, "=") {
		return "#" + fragment
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String()
}
func navigationURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.RawQuery = ""
	u.User = nil
	u.Fragment = strings.SplitN(u.Fragment, "?", 2)[0]
	if strings.Contains(u.Fragment, "=") {
		u.Fragment = ""
	}
	return u.String()
}

// Chrome requires a writable native path; do not rely on it creating a
// missing directory or interpreting POSIX separators on Windows.
func (e *engine) downloadDir(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", errors.New("invalid_argument")
	}
	parent := filepath.Join(e.dir, "downloads")
	if err := os.Mkdir(parent, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	if actual, err := filepath.EvalSymlinks(parent); err != nil || actual != parent {
		return "", errors.New("storage_failed")
	}
	dir := filepath.Join(parent, id)
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	actual, err := filepath.EvalSymlinks(dir)
	if err != nil || actual != dir {
		return "", errors.New("storage_failed")
	}
	return dir, nil
}
