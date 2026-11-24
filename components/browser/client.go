package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

// Rod still initializes a complete Page (including its private JS/keyboard
// state), but it shares our auto-attached session instead of creating another
// debugger that may keep the renderer paused. Session lifetime stays here.
type sessionClient struct {
	base  rod.CDPClient
	owner *session
}

func (c *sessionClient) Event() <-chan *cdp.Event { return c.base.Event() }
func (c *sessionClient) Call(ctx context.Context, sid, method string, params any) ([]byte, error) {
	c.owner.trace(method, proto.TargetSessionID(sid), false)
	defer c.owner.trace(method, proto.TargetSessionID(sid), true)
	switch method {
	case "Browser.close":
		return nil, errors.New("unsupported")
	case "Target.attachToTarget":
		var in struct {
			TargetID proto.TargetTargetID `json:"targetId"`
		}
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		c.owner.mu.Lock()
		installed := c.owner.installs[in.TargetID]
		c.owner.mu.Unlock()
		if installed != nil {
			if installed.configured != nil {
				select {
				case <-installed.configured:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			select {
			case <-installed.ready:
				if installed.err != nil {
					return nil, installed.err
				}
			default:
			}
			return json.Marshal(map[string]proto.TargetSessionID{"sessionId": installed.sid})
		}
		c.owner.trace("Target.attachToTarget:forwarded", proto.TargetSessionID(sid), true)
	case "Target.detachFromTarget":
		var in struct {
			SessionID proto.TargetSessionID `json:"sessionId"`
			TargetID  proto.TargetTargetID  `json:"targetId"`
		}
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		c.owner.mu.Lock()
		owned := false
		for id, installed := range c.owner.installs {
			if (in.SessionID != "" && installed.sid == in.SessionID) || (in.TargetID != "" && id == in.TargetID) {
				owned = true
				break
			}
		}
		c.owner.mu.Unlock()
		if owned {
			return []byte(`{}`), nil
		}
	}
	c.owner.mu.Lock()
	operation := c.owner.commandCtx
	c.owner.mu.Unlock()
	if operation != nil && ctx.Value(engineInternal{}) != true {
		bounded, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(operation, cancel)
		defer stop()
		defer cancel()
		if operation.Err() != nil {
			cancel()
		}
		ctx = bounded
	}

	return c.base.Call(ctx, sid, method, params)
}
