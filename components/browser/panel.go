package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

//go:embed panel.js
var panelSource string

func (s *session) collectorScript() string {
	if s.headless {
		return recorderSource
	}
	return recorderSource + "\n" + panelSource
}

var panelVariable = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

type panelStep struct {
	Clip   *screenshotClip `json:"clip,omitempty"`
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Title  string          `json:"title"`
	Target string          `json:"target"`
	What   string          `json:"what,omitempty"`
	Into   string          `json:"into,omitempty"`
}
type panelState struct {
	Features     []string    `json:"features"`
	Hide         bool        `json:"hide"`
	Revision     uint64      `json:"revision"`
	ID           string      `json:"id"`
	Recording    bool        `json:"recording"`
	Busy         bool        `json:"busy"`
	Picking      bool        `json:"picking"`
	Selected     string      `json:"selected"`
	NextVariable string      `json:"next_variable"`
	Steps        []panelStep `json:"steps"`
	Error        string      `json:"error"`
}

func targetSummary(t *target) string {
	if t == nil {
		return ""
	}
	for _, l := range t.Locators {
		if l.Name != "" {
			return truncate(l.Name, 100)
		}
		if l.Value != "" {
			return truncate(l.Value, 100)
		}
	}
	return ""
}
func (s *session) recordingPanelState(code string) panelState {
	s.panelMu.Lock()
	defer s.panelMu.Unlock()
	s.panelRevision++
	s.mu.Lock()
	rec, controller := s.recorder, s.controller
	s.mu.Unlock()
	out := panelState{Features: []string{"screenshot_clip"}, Revision: s.panelRevision, Hide: controller != "" && !strings.HasPrefix(controller, "rec_"), Busy: controller != "", Steps: []panelStep{}, Error: code}
	if rec == nil {
		return out
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	out.ID, out.Recording, out.Picking = rec.id, !rec.done && !rec.closing, rec.picking && !rec.done && !rec.closing
	out.NextVariable = "data" + strconv.Itoa(rec.extractCount+1)
	if rec.selected != nil && out.Recording {
		out.Selected = targetSummary(rec.selected.node.Target)
	}
	for _, n := range rec.steps {
		if !rec.removed[n.ID] {
			out.Steps = append(out.Steps, panelStep{Clip: n.Clip, ID: n.ID, Type: n.Type, Title: n.Title, Target: targetSummary(n.Target), What: n.What, Into: n.Into})
		}
	}
	return out
}
func (s *session) updatePanel(code string) error {
	raw, _ := json.Marshal(s.recordingPanelState(code))
	s.mu.Lock()
	contexts := map[proto.TargetSessionID][]proto.RuntimeExecutionContextID{}
	for sid, ids := range s.contexts {
		for id := range ids {
			contexts[proto.TargetSessionID(sid)] = append(contexts[proto.TargetSessionID(sid)], id)
		}
	}
	s.mu.Unlock()
	var first error
	for sid, ids := range contexts {
		for _, id := range ids {
			if err := s.call(sid, "Runtime.evaluate", map[string]any{"expression": "if(window.__mycoRecorder)window.__mycoRecorder.updatePanel(" + string(raw) + ")", "contextId": id}, nil); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// Controls arrive only through the isolated collector binding. Site-world
// scripts cannot call it. Start/stop/delete/form controls are top-frame only;
// selection messages can come from a child frame and retain that frame path.
func (s *session) panelCommand(sid proto.TargetSessionID, frame proto.PageFrameID, raw string) {
	var in struct {
		Action      string          `json:"action"`
		RecordingID string          `json:"recording_id"`
		ID          string          `json:"id"`
		Target      *target         `json:"target"`
		What        string          `json:"what"`
		Attr        string          `json:"attr"`
		Into        string          `json:"into"`
		Clip        *screenshotClip `json:"clip"`
	}
	if json.Unmarshal([]byte(raw), &in) != nil {
		return
	}
	s.mu.Lock()
	rec, p := s.recorder, s.pages[s.targets[sid]]
	if p == nil {
		p = s.pages[s.current]
	}
	top := frame != "" && s.frames[frame].Parent == "" && s.pages[s.targets[sid]] != nil
	s.mu.Unlock()
	if !top && in.Action != "picked" && in.Action != "cancel-pick" {
		return
	}
	if in.Action != "start" && (rec == nil || in.RecordingID != rec.id) {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	var err error
	switch in.Action {
	case "start":
		if p == nil {
			err = errors.New("target_not_found")
			break
		}
		info, e := p.Context(ctx).Info()
		if e != nil {
			err = e
			break
		}
		_, err = s.e.startRecording(ctx, s, p, info.URL, "browser")
	case "stop":
		if rec != nil {
			rec.flush(ctx)
			rec.finish()
		}
	case "remove":
		if rec != nil {
			rec.remove(in.ID)
		}
	case "pick":
		if rec != nil {
			rec.mu.Lock()
			if !rec.closing {
				rec.picking = true
				rec.selected = nil
			}
			rec.mu.Unlock()
		}
	case "cancel-pick":
		if rec != nil {
			rec.mu.Lock()
			rec.picking = false
			rec.selected = nil
			rec.mu.Unlock()
		}
	case "picked":
		if rec == nil || p == nil || in.Target == nil || !recordable(node{Type: "click", Target: in.Target}) {
			return
		}
		rec.mu.Lock()
		if rec.picking && !rec.closing {
			rec.picking = false
			rec.selected = &captured{node: node{Target: in.Target}, page: p, frame: frame}
		}
		rec.mu.Unlock()
	case "screenshot":
		if rec == nil || p == nil || !in.Clip.valid() {
			err = errors.New("invalid_argument")
			break
		}
		rec.mu.Lock()
		active := !rec.closing && !rec.done && !rec.picking
		rec.mu.Unlock()
		if active {
			rec.enqueue(node{Type: "screenshot", Clip: in.Clip}, p)
		}
	case "extract-target":
		// A top-frame trusted context-menu choice supplies its own target. Keep
		// target, format and variable in one command rather than racing a pick
		// acknowledgement against a second extraction command.
		if rec == nil || p == nil || in.Target == nil || !recordable(node{Type: "click", Target: in.Target}) || (in.What != "text" && in.What != "attribute" && in.What != "html") || len(in.Attr) > 160 || (in.What == "attribute" && in.Attr == "") || (in.Into != "" && !panelVariable.MatchString(in.Into)) {
			err = errors.New("invalid_argument")
			break
		}
		rec.mu.Lock()
		active := !rec.closing && !rec.done && !rec.picking
		if active {
			rec.extractCount++
			if in.Into == "" {
				in.Into = "data" + strconv.Itoa(rec.extractCount)
			}
			rec.selected = nil
		}
		rec.mu.Unlock()
		if active {
			rec.enqueueFrame(node{Type: "extract", Target: in.Target, What: in.What, Attr: in.Attr, Into: in.Into}, p, frame)
		}
	case "extract":
		if rec == nil || !panelVariable.MatchString(in.Into) || (in.What != "text" && in.What != "attribute" && in.What != "html") || len(in.Attr) > 160 || (in.What == "attribute" && in.Attr == "") {
			err = errors.New("invalid_argument")
			break
		}
		rec.mu.Lock()
		selected := rec.selected
		rec.selected = nil
		if selected != nil && !rec.closing {
			rec.extractCount++
		} else {
			selected = nil
		}
		rec.mu.Unlock()
		if selected != nil {
			n := selected.node
			n.Type = "extract"
			n.What = in.What
			n.Attr = in.Attr
			n.Into = in.Into
			rec.enqueueFrame(n, selected.page, selected.frame)
		}
	default:
		return
	}
	code := ""
	if err != nil {
		code = publicError(err)
	}
	s.updatePanel(code)
}
