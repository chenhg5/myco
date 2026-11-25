package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

func (r *playRun) act(ctx context.Context, n node) (any, error) {
	s := r.session
	restore := s.commands(ctx)
	defer restore()
	p, err := s.page()
	if err != nil {
		return nil, err
	}
	p = p.Context(ctx)
	if n.Type == "navigate" && n.Target != nil && len(n.Target.Frame) > 0 {
		p, err = targetPage(ctx, p, *n.Target)
		if err != nil {
			return nil, err
		}
	}
	var el *rod.Element
	need := map[string]bool{"click": true, "type": true, "select": true, "hover": true, "drag": true, "upload": true, "extract": true}
	if n.Target != nil && len(n.Target.Locators) > 0 && n.Type != "assert" {
		el, err = findTarget(ctx, p, *n.Target)
		if err != nil {
			return nil, err
		}
	}
	if need[n.Type] && el == nil {
		return nil, errors.New("target_not_found")
	}
	switch n.Type {
	case "note", "confirm":
	case "dialog":
		return nil, r.dialog(ctx, n)
	case "navigate":
		if checkedURL(n.URL) != nil {
			return nil, errors.New("invalid_argument")
		}
		err = p.Navigate(n.URL)
		if err == nil {
			err = p.WaitLoad()
		}

	case "back", "forward":
		history, e := (proto.PageGetNavigationHistory{}).Call(p)
		if e != nil {
			return nil, e
		}
		index := history.CurrentIndex - 1
		if n.Type == "forward" {
			index = history.CurrentIndex + 1
		}
		if index < 0 || index >= len(history.Entries) {
			return nil, errors.New("target_not_found")
		}
		err = (proto.PageNavigateToHistoryEntry{EntryID: history.Entries[index].ID}).Call(p)
	case "reload":
		err = (proto.PageReload{}).Call(p)
	case "click":
		count := max(1, min(2, n.Count))
		button := proto.InputMouseButtonLeft
		if n.Button == "right" {
			button = proto.InputMouseButtonRight
		}
		if n.Button == "middle" {
			button = proto.InputMouseButtonMiddle
		}
		err = el.Click(button, count)
	case "type":
		err = el.Focus()
		if err == nil && n.Clear {
			err = el.SelectAllText()
			if err == nil {
				err = pressKeys(p, "Backspace")
			}
		}
		if err == nil {
			err = el.Input(n.Value)
		}
	case "select":
		err = el.Focus()
		if err == nil {
			_, err = el.Eval(`function(v){const o=[...this.options].find(o=>o.value===v)||[...this.options].find(o=>o.textContent.trim()===v);if(!o||o.disabled||this.disabled)throw Error('not_found');this.value=o.value;this.dispatchEvent(new Event('input',{bubbles:true}));this.dispatchEvent(new Event('change',{bubbles:true}))}`, n.Value)
		}
	case "press":
		if el != nil {
			err = el.Focus()
			if err != nil {
				return nil, err
			}
		}
		err = pressKeys(p, n.Key)
	case "hover":
		err = el.Hover()
	case "drag":
		if n.To == nil {
			return nil, errors.New("invalid_argument")
		}
		to, e := findTarget(ctx, p, *n.To)
		if e != nil {
			return nil, e
		}
		fromPoint, e := el.WaitInteractable()
		if e != nil {
			return nil, e
		}
		toPoint, e := to.WaitInteractable()
		if e != nil {
			return nil, e
		}
		if err = p.Mouse.MoveTo(*fromPoint); err == nil {
			err = p.Mouse.Down(proto.InputMouseButtonLeft, 1)
			if err == nil {
				defer p.Mouse.Up(proto.InputMouseButtonLeft, 1)
				err = p.Mouse.MoveLinear(*toPoint, 8)
			}
		}
	case "scroll":
		if el != nil {
			_, err = el.Eval(`function(y,absolute){this.scrollTo(0,absolute?y:this.scrollTop+y)}`, n.DY, n.Value == "absolute")
		} else {
			_, err = p.Eval(`(y,absolute)=>window.scrollTo(0,absolute?y:scrollY+y)`, n.DY, n.Value == "absolute")
		}
	case "upload":
		path, e := filepath.EvalSymlinks(n.Value)
		if e != nil {
			return nil, errors.New("invalid_argument")
		}
		allowed := false
		for _, pick := range r.uploads {
			if path == pick {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, errors.New("permission_denied")
		}
		fi, e := os.Stat(path)
		if e != nil || !fi.Mode().IsRegular() || fi.Size() > 64<<20 {
			return nil, errors.New("invalid_argument")
		}
		err = el.SetFiles([]string{path})
	case "wait":
		switch n.Wait {
		case "url":
			if n.Value == "" {
				return nil, errors.New("invalid_argument")
			}
			err = p.Wait(rod.Eval(`u=>location.href.includes(u)`, n.Value))
		case "download":
			return r.download(ctx)
		case "time":
			if n.MS < 1 || n.MS > 600000 {
				return nil, errors.New("invalid_argument")
			}
			timer := time.NewTimer(time.Duration(n.MS) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-timer.C:
			}
		case "network":
			p.WaitRequestIdle(300*time.Millisecond, nil, nil, nil)()
			err = ctx.Err()
		default:
			if el == nil {
				return nil, errors.New("target_not_found")
			}
			err = el.WaitVisible()
		}
	case "assert":
		switch n.Check {
		case "url":
			result, e := p.Eval(`u=>location.href.includes(u)`, n.Value)
			err = e
			if e == nil && !result.Value.Bool() {
				err = errors.New("assertion_failed")
			}
		case "expr":
			result, e := p.Eval("()=>Boolean(" + n.Code + ")")
			err = e
			if e == nil && !result.Value.Bool() {
				err = errors.New("assertion_failed")
			}
		case "hidden":
			if n.Target == nil {
				return nil, errors.New("invalid_argument")
			}
			frame, e := targetPage(ctx, p, *n.Target)
			if e != nil {
				return nil, e
			}
			err = frame.Wait(rod.Eval(`t=>{const query=(`+locatorSource+`);return query(t,true).every(e=>e.getBoundingClientRect().width===0||e.getBoundingClientRect().height===0||getComputedStyle(e).visibility==='hidden')}`, *n.Target))
		default:
			if n.Target == nil {
				return nil, errors.New("invalid_argument")
			}
			el, err = findTarget(ctx, p, *n.Target)
			if err == nil && n.Check == "text" {
				text, e := el.Text()
				err = e
				if e == nil && !strings.Contains(text, n.Value) {
					err = errors.New("assertion_failed")
				}
			}
		}
	case "extract":
		switch n.What {
		case "html":
			return el.HTML()
		case "attribute":
			return el.Attribute(n.Attr)
		case "table":
			result, e := el.Eval(`function(){return [...this.querySelectorAll('tr')].map(r=>[...r.querySelectorAll('th,td')].map(c=>c.innerText))}`)
			if e != nil {
				return nil, e
			}
			return result.Value.Val(), nil
		case "js":
			result, e := el.Eval("function(){" + n.Code + "}")
			if e != nil {
				return nil, e
			}
			return result.Value.Val(), nil
		default:
			return el.Text()
		}
	case "js":
		result, e := p.Eval("()=>{" + n.Code + "}")
		if e != nil {
			return nil, e
		}
		return result.Value.Val(), nil
	case "screenshot":
		if n.Full {
			if len(r.secrets) > 0 {
				return nil, errors.New("sensitive_snapshot")
			}
			return s.e.fullSnapshot(r.scope, p)
		}
		if n.Clip != nil {
			if len(r.secrets) > 0 {
				return nil, errors.New("sensitive_snapshot")
			}
			return s.e.clipSnapshot(r.scope, p, n.Clip)
		}
		id, e := s.e.snapshot(r.scope, p, n.Target)
		return id, e
	case "tab":
		return r.tab(ctx, n)

	default:
		return nil, errors.New("unsupported_node")
	}
	return nil, err
}
func keyEvents(text string) (*proto.InputDispatchKeyEvent, *proto.InputDispatchKeyEvent, error) {
	parts := strings.Split(text, "+")
	key := parts[len(parts)-1]
	parts = parts[:len(parts)-1]
	if key == "" && strings.HasSuffix(text, "+") {
		key = "+"
		if len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
	}
	if len(parts) > 4 {
		return nil, nil, errors.New("invalid_argument")
	}
	modifiers := 0
	for _, m := range parts {
		switch m {
		case "Alt":
			modifiers |= 1
		case "Control", "Ctrl":
			modifiers |= 2
		case "Meta", "Command":
			modifiers |= 4
		case "Shift":
			modifiers |= 8
		default:
			return nil, nil, errors.New("invalid_argument")
		}
	}
	keys := map[string]input.Key{"Enter": input.Enter, "Tab": input.Tab, "Escape": input.Escape, "Backspace": input.Backspace, "Delete": input.Delete, "ArrowLeft": input.ArrowLeft, "ArrowRight": input.ArrowRight, "ArrowUp": input.ArrowUp, "ArrowDown": input.ArrowDown, "Home": input.Home, "End": input.End, "PageUp": input.PageUp, "PageDown": input.PageDown, "Insert": input.Insert}
	k, ok := keys[key]
	if !ok {
		if len(key) != 1 || key[0] < 32 || key[0] > 126 {
			return nil, nil, errors.New("invalid_argument")
		}
		k = input.Key(key[0])
	}
	down, up := k.Encode(proto.InputDispatchKeyEventTypeKeyDown, modifiers), k.Encode(proto.InputDispatchKeyEventTypeKeyUp, modifiers)
	if modifiers&7 != 0 {
		down.Text = ""
		down.UnmodifiedText = ""
	}
	if key == "Enter" && modifiers&7 == 0 {
		down.Text = "\r"
		down.UnmodifiedText = "\r"
	}
	up.Text = ""
	up.UnmodifiedText = ""
	return down, up, nil
}
func pressKeys(p *rod.Page, text string) error {
	down, up, err := keyEvents(text)
	if err != nil {
		return err
	}
	if err = down.Call(p); err != nil {
		return err
	}
	return up.Call(p)
}
