package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type preparedDialog struct {
	gated     bool
	stepAfter bool
	err       error
}
type actionResult struct {
	value any
	err   error
}

func (s *session) waitPage(ctx context.Context, id proto.TargetTargetID) (*rod.Page, error) {
	for {
		s.mu.Lock()
		p := s.pages[id]
		installed := s.installs[id]
		changed := s.observed
		if changed == nil {
			s.observed = make(chan struct{})
			changed = s.observed
		}
		s.mu.Unlock()
		if installed != nil {
			select {
			case <-installed.ready:
				if installed.err != nil {
					return nil, installed.err
				}
				s.mu.Lock()
				p = s.pages[id]
				s.mu.Unlock()
				if p != nil {
					return p, nil
				}
				return nil, errors.New("target_not_found")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if p != nil {
			return p, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}
func (s *session) newPage(ctx context.Context, rawURL string) (*rod.Page, error) {
	s.mu.Lock()
	count := len(s.pages)
	s.mu.Unlock()
	if count >= 32 {
		return nil, errors.New("tab_limit")
	}
	var created struct {
		ID proto.TargetTargetID `json:"targetId"`
	}
	data, err := s.browser.Call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &created); err != nil {
		return nil, err
	}
	p, err := s.waitPage(ctx, created.ID)
	failed := true
	defer func() {
		if failed {
			cleanup, stop := context.WithTimeout(s.ctx, 2*time.Second)
			defer stop()
			_, _ = s.browser.Call(cleanup, "", "Target.closeTarget", map[string]any{"targetId": created.ID})
		}
	}()
	if err != nil {
		return nil, err
	}
	if rawURL != "" && rawURL != "about:blank" {
		if checkedURL(rawURL) != nil {
			return nil, errors.New("invalid_argument")
		}
		if err = p.Context(ctx).Navigate(rawURL); err != nil {
			return nil, err
		}
		if err = p.Context(ctx).WaitLoad(); err != nil {
			return nil, err
		}
	}
	if _, err = p.Context(ctx).Activate(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.current = p.TargetID
	s.mu.Unlock()
	failed = false
	return p, nil
}
func (r *playRun) actionAnchor(ctx context.Context, n node) error {
	p, err := r.session.page()
	if err != nil {
		return err
	}
	if n.Target != nil && len(n.Target.Frame) > 0 {
		p, err = targetPage(ctx, p, *n.Target)
		if err != nil {
			return err
		}
	}
	r.after = r.session.cursor()
	r.anchorTarget = p.TargetID
	r.anchorFrame = p.FrameID
	return nil
}
func sideEffect(n node) bool {
	return n.Type == "wait" || n.Type == "dialog" || n.Type == "assert" || n.Type == "screenshot" || n.Type == "note" || (n.Type == "tab" && n.Tab != "new")
}
func (r *playRun) perform(ctx context.Context, n node, next *node, timeout time.Duration) (any, error) {
	if r.session == nil {
		return r.action(ctx, n)
	}
	if !sideEffect(n) {
		if err := r.actionAnchor(ctx, n); err != nil {
			return nil, err
		}
	}
	if next == nil || next.Type != "dialog" || n.Type == "dialog" {
		return r.action(ctx, n)
	}
	// Chrome can hold a click/JS command until its dialog is handled. Arm the
	// next explicit dialog step first, while keeping its breakpoint observable.
	prepared, err := r.prepare(*next)
	if err != nil {
		return nil, err
	}
	if r.preparedDialogs == nil {
		r.preparedDialogs = map[string]*preparedDialog{}
	}
	pending := &preparedDialog{}
	r.preparedDialogs[next.ID] = pending
	actionCtx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	actionDone := make(chan actionResult, 1)
	go func() { value, err := r.action(actionCtx, n); actionDone <- actionResult{value, err} }()
	observationCtx, stop := context.WithTimeout(r.ctx, timeout)
	defer stop()
	observed := make(chan struct {
		event observation
		err   error
	}, 1)
	go func() {
		event, err := r.session.waitObserved(observationCtx, r.after, func(evt observation) bool { return evt.Kind == "dialog" && evt.Target == r.anchorTarget })
		observed <- struct {
			event observation
			err   error
		}{event, err}
	}()
	var result *actionResult
	for {
		select {
		case value := <-actionDone:
			result = &value
			if value.err != nil {
				pending.err = value.err
				return value.value, value.err
			}
		case found := <-observed:
			if found.err != nil {
				pending.err = found.err
				return nil, found.err
			}
			stop()
			r.mu.Lock()
			if r.step {
				r.paused = true
				r.step = false
			}
			r.mu.Unlock()
			if err = r.gate(prepared); err != nil {
				pending.err = err
				return nil, err
			}
			pending.gated = true
			r.mu.Lock()
			pending.stepAfter = r.step
			r.step = false
			r.mu.Unlock()
			handling, finish := context.WithTimeout(r.ctx, timeout)
			err = r.handleDialog(handling, prepared, found.event)
			finish()
			pending.err = err
			if err != nil {
				return nil, err
			}
			if result != nil {
				return result.value, result.err
			}
			completion := time.NewTimer(timeout)
			defer completion.Stop()
			select {
			case value := <-actionDone:
				return value.value, value.err
			case <-r.ctx.Done():
				return nil, r.ctx.Err()
			case <-completion.C:
				return nil, context.DeadlineExceeded
			}
		case <-r.ctx.Done():
			return nil, r.ctx.Err()
		}
	}
}
func (r *playRun) handleDialog(ctx context.Context, n node, evt observation) error {
	if r.applyDialog != nil {
		return r.applyDialog(ctx, n, evt)
	}
	if n.Dialog != "accept" && n.Dialog != "dismiss" {
		return errors.New("invalid_argument")
	}
	_, err := r.session.browser.Call(ctx, string(evt.Session), "Page.handleJavaScriptDialog", map[string]any{"accept": n.Dialog == "accept", "promptText": n.Value})
	return err
}
func (r *playRun) dialog(ctx context.Context, n node) error {
	if prepared := r.preparedDialogs[n.ID]; prepared != nil {
		return prepared.err
	}
	evt, err := r.session.waitObserved(ctx, r.after, func(evt observation) bool { return evt.Kind == "dialog" && evt.Target == r.anchorTarget })
	if err != nil {
		return err
	}
	return r.handleDialog(ctx, n, evt)
}
func (r *playRun) download(ctx context.Context) (string, error) {
	start, err := r.session.waitObserved(ctx, r.after, func(evt observation) bool { return evt.Kind == "download" })
	if err != nil {
		return "", err
	}
	done, err := r.session.waitObserved(ctx, start.Sequence-1, func(evt observation) bool { return evt.Kind == "download-complete" && evt.ID == start.ID })
	if err != nil {
		return "", err
	}
	if done.Error != "" {
		return "", errors.New(done.Error)
	}
	// A shared result contains a filename, never a private absolute path.
	return filepath.Base(done.Filename), nil
}
func (r *playRun) tab(ctx context.Context, n node) (any, error) {
	s := r.session
	switch n.Tab {
	case "new":
		p, err := s.newPage(ctx, n.URL)
		if err != nil {
			return nil, err
		}
		r.tabOrder = append(r.tabOrder, p.TargetID)
		return nil, nil
	case "popup":
		event, err := s.waitObserved(ctx, r.after, func(evt observation) bool {
			return evt.Kind == "target" && (evt.Opener == r.anchorTarget || (r.anchorFrame != "" && evt.OpenerFrame == r.anchorFrame))
		})
		if err != nil {
			return nil, err
		}
		p, err := s.waitPage(ctx, event.Target)
		if err != nil {
			return nil, err
		}
		exists := false
		for _, id := range r.tabOrder {
			if id == p.TargetID {
				exists = true
			}
		}
		if !exists {
			r.tabOrder = append(r.tabOrder, p.TargetID)
		}
		s.mu.Lock()
		s.current = p.TargetID
		s.mu.Unlock()
		_, err = p.Context(ctx).Activate()
		return nil, err
	case "opener":
		p, err := s.page()
		if err != nil {
			return nil, err
		}
		info, err := p.Context(ctx).Info()
		if err != nil || info.OpenerID == "" {
			return nil, errors.New("target_not_found")
		}
		opener, err := s.waitPage(ctx, info.OpenerID)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.current = opener.TargetID
		s.mu.Unlock()
		_, err = opener.Context(ctx).Activate()
		return nil, err
	default:
		index, err := strconv.Atoi(n.Tab)
		if err != nil || index < 0 || index > 99 || index >= len(r.tabOrder) {
			return nil, errors.New("target_not_found")
		}
		p, err := s.waitPage(ctx, r.tabOrder[index])
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.current = p.TargetID
		s.mu.Unlock()
		_, err = p.Context(ctx).Activate()
		return nil, err
	}
}
