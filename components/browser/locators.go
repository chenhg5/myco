package main

import (
	"context"
	_ "embed"
	"errors"
	"sort"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

//go:embed locators.js
var locatorSource string

func targetPage(ctx context.Context, p *rod.Page, t target) (*rod.Page, error) {
	p = p.Context(ctx)
	if len(t.Frame) > 16 || len(t.Shadow) > 16 || len(t.Locators) > 12 {
		return nil, errors.New("invalid_argument")
	}
	for _, selector := range t.Frame {
		matches, err := p.Elements(selector)
		if err != nil {
			return nil, err
		}
		if len(matches) != 1 {
			return nil, errors.New("target_not_found")
		}
		el := matches[0]
		described, err := el.Describe(1, true)
		if err != nil {
			return nil, err
		}
		if described.ContentDocument == nil && described.FrameID != "" {
			p, err = p.Browser().PageFromTarget(proto.TargetTargetID(described.FrameID))
		} else {
			p, err = el.Frame()
		}
		if err != nil {
			return nil, err
		}
		p = p.Context(ctx)
	}
	return p, nil
}
func findTarget(ctx context.Context, p *rod.Page, t target) (*rod.Element, error) {
	frame, err := targetPage(ctx, p, t)
	if err != nil {
		return nil, err
	}
	el, err := frame.ElementByJS(rod.Eval(locatorSource, t, false))
	if err != nil {
		return nil, errors.New("target_not_found")
	}
	return el, nil
}
func (s *session) frameChain(p *rod.Page, id proto.PageFrameID) ([]string, error) {
	if p == nil {
		return nil, errors.New("frame_unavailable")
	}
	if id == "" || id == p.FrameID {
		return nil, nil
	}
	var tree struct {
		FrameTree *structFrameTree `json:"frameTree"`
	}
	_ = s.call(p.SessionID, "Page.getFrameTree", map[string]any{}, &tree)
	parents := map[proto.PageFrameID]proto.PageFrameID{}
	var walk func(*structFrameTree, proto.PageFrameID)
	walk = func(n *structFrameTree, parent proto.PageFrameID) {
		if n == nil {
			return
		}
		parents[n.Frame.ID] = parent
		for _, child := range n.Children {
			walk(child, n.Frame.ID)
		}
	}
	walk(tree.FrameTree, "")
	chain := []proto.PageFrameID{}
	seen := map[proto.PageFrameID]bool{}
	for fid := id; fid != "" && fid != p.FrameID; fid = parents[fid] {
		if seen[fid] || len(chain) >= 16 {
			return nil, errors.New("frame_unavailable")
		}
		seen[fid] = true
		chain = append(chain, fid)
		if _, ok := parents[fid]; !ok {
			return nil, errors.New("frame_unavailable")
		}
	}
	s.mu.Lock()
	sessions := []proto.TargetSessionID{}
	for sid := range s.targets {
		sessions = append(sessions, sid)
	}
	s.mu.Unlock()
	sort.Slice(sessions, func(i, j int) bool { return sessions[i] < sessions[j] })
	out := []string{}
	for i := len(chain) - 1; i >= 0; i-- {
		selector := ""
		for _, sid := range sessions {
			var owner struct {
				Backend proto.DOMBackendNodeID `json:"backendNodeId"`
			}
			if s.call(sid, "DOM.getFrameOwner", map[string]any{"frameId": chain[i]}, &owner) != nil {
				continue
			}
			described, _, describeErr := s.describeBackend(sid, owner.Backend)
			if describeErr != nil || len(described.Shadow) > 0 {
				continue
			}
			for _, l := range described.Locators {
				if l.Kind == "css" {
					selector = l.Value
					break
				}
			}
			if selector != "" {
				break
			}
		}

		if selector == "" {
			return nil, errors.New("frame_unavailable")
		}
		out = append(out, selector)
	}
	return out, nil
}

type structFrameTree struct {
	Frame    proto.PageFrame    `json:"frame"`
	Children []*structFrameTree `json:"childFrames"`
}
