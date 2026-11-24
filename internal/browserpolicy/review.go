package browserpolicy

import (
	"encoding/json"
	"fmt"
	"github.com/myco-ai/myco/internal/api"
	"sort"
	"strings"
	"time"
)

// InvalidGraph identifies a rejected executable field without exposing code.
type InvalidGraph struct{ Node string }

func (e *InvalidGraph) Error() string {
	node := e.Node
	if len(node) > 80 {
		node = node[:80]
	}
	return fmt.Sprintf("invalid_graph at node %q: code must be fixed text without ${...}", node)
}

// CheckFixedCode applies to human and agent runs. Website-extracted values are
// as untrusted as agent params; neither may be substituted into executable JS.
func CheckFixedCode(raw json.RawMessage) error {
	var script struct {
		Graph struct {
			Nodes []struct {
				ID   string `json:"id"`
				Code string `json:"code"`
			} `json:"nodes"`
		} `json:"graph"`
	}
	if err := json.Unmarshal(raw, &script); err != nil {
		return err
	}
	for _, n := range script.Graph.Nodes {
		if strings.Contains(n.Code, "${") {
			return &InvalidGraph{Node: n.ID}
		}
	}
	return nil
}

type ReviewedCode = api.BrowserReviewedCode
type Review = api.BrowserApprovalReview

// Prepare validates and canonicalizes the exact data shown for owner consent.
func Prepare(scope Scope, req Request) (Review, error) {
	snap, err := snapshot(req, scope.Page, time.Time{})
	if err != nil {
		return Review{}, err
	}
	if err := checkChildren(req.Script, req.Scripts); err != nil {
		return Review{}, err
	}
	out := Review{ScriptID: req.ScriptID, Version: req.Version, Profile: req.Profile, Name: snap.Approval.Name, Digest: snap.Approval.Digest, URLs: []string{}, Code: []ReviewedCode{}, Children: []string{}, Script: snap.Script, Scripts: snap.Scripts}
	inspect := func(id string, raw json.RawMessage) error {
		var script struct {
			StartURL string                                 `json:"start_url"`
			Params   []struct{ Name, Type, Default string } `json:"params"`
			Graph    struct {
				Nodes []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					URL      string `json:"url"`
					Code     string `json:"code"`
					ScriptID string `json:"script_id"`
				} `json:"nodes"`
			} `json:"graph"`
		}
		// The actual graph remains raw and is validated by the installed component
		// before execution. This pass enforces the native approval trust boundary.
		if err := json.Unmarshal(raw, &script); err != nil {
			return err
		}
		if script.StartURL != "" {
			out.URLs = append(out.URLs, script.StartURL)
		}
		for _, p := range script.Params {
			if p.Type == "secret" && p.Default != "" {
				return fmt.Errorf("secret defaults cannot be approved")
			}
		}
		for _, n := range script.Graph.Nodes {
			if n.URL != "" {
				out.URLs = append(out.URLs, n.URL)
			}
			if n.Code != "" {
				if strings.Contains(n.Code, "${") {
					return &InvalidGraph{Node: n.ID}
				}
				out.Code = append(out.Code, ReviewedCode{Script: id, Node: n.ID, Type: n.Type, Code: n.Code})
			}
		}
		return nil
	}
	if err = inspect(req.ScriptID, snap.Script); err != nil {
		return Review{}, err
	}
	for id := range snap.Scripts {
		out.Children = append(out.Children, id)
	}
	sort.Strings(out.Children)
	for _, id := range out.Children {
		if err = inspect(id, snap.Scripts[id]); err != nil {
			return Review{}, err
		}
	}
	return out, nil
}

// Check the transitive closure, including nested references. Unreferenced
// children are rejected so the consent surface matches execution exactly.
func checkChildren(root json.RawMessage, children map[string]json.RawMessage) error {
	seen := map[string]bool{}
	var visit func(json.RawMessage) error
	visit = func(raw json.RawMessage) error {
		var s struct {
			Graph struct {
				Nodes []struct {
					Type     string `json:"type"`
					ScriptID string `json:"script_id"`
				} `json:"nodes"`
			} `json:"graph"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		for _, n := range s.Graph.Nodes {
			if n.Type != "subflow" {
				continue
			}
			child, ok := children[n.ScriptID]
			if !ok || n.ScriptID == "" {
				return fmt.Errorf("missing subflow snapshot")
			}
			if !seen[n.ScriptID] {
				seen[n.ScriptID] = true
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return err
	}
	if len(seen) != len(children) {
		return fmt.Errorf("unreferenced subflow snapshot")
	}
	return nil
}
