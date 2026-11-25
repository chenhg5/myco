package browserautomation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/myco-ai/myco/internal/api"
)

// ErrUnsupportedNode means capability negotiation rejected the run before its POST.
var ErrUnsupportedNode = errors.New("unsupported_node")

// Check the main and child scripts before handing a run to an older component.
// A clip requires support even when its value is null or malformed.
func runRequiredFeatures(raw []byte) []string {
	var envelope struct {
		Script  json.RawMessage            `json:"script"`
		Scripts map[string]json.RawMessage `json:"scripts"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return nil
	}
	scripts := []json.RawMessage{envelope.Script}
	for _, script := range envelope.Scripts {
		scripts = append(scripts, script)
	}
	clip, control := false, false
	for _, script := range scripts {
		var value struct {
			Graph struct {
				Nodes []map[string]json.RawMessage `json:"nodes"`
			} `json:"graph"`
		}
		if json.Unmarshal(script, &value) != nil {
			continue
		}
		for _, node := range value.Graph.Nodes {
			clip = clip || node["clip"] != nil
			var kind, tab string
			var full bool
			_ = json.Unmarshal(node["type"], &kind)
			_ = json.Unmarshal(node["tab"], &tab)
			_ = json.Unmarshal(node["full"], &full)
			control = control || kind == "back" || kind == "forward" || kind == "reload" ||
				kind == "tab" && tab == "close" || kind == "screenshot" && full
		}
	}
	var required []string
	if clip {
		required = append(required, "screenshot_clip")
	}
	if control {
		required = append(required, api.BrowserControlFeature)
	}
	return required
}

func requireBrowserFeatures(ctx context.Context, hc *http.Client, address, secret, scope string, required ...string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+address+"/status", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("X-Myco-Scope", scope)
	response, err := hc.Do(req)
	if err != nil {
		return errors.New("component_unavailable")
	}
	defer response.Body.Close()
	var status struct {
		Features []string `json:"features"`
		Error    string   `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&status) != nil {
		return errors.New("component_failed")
	}
	if response.StatusCode >= 400 {
		if status.Error != "" {
			return errors.New(status.Error)
		}
		return errors.New("component_failed")
	}
	for _, feature := range required {
		if !slices.Contains(status.Features, feature) {
			return ErrUnsupportedNode
		}
	}
	return nil
}

// Keep the preflight tied to the very same process endpoint/credential as the
// run POST; checking c.Call recursively could race a component replacement.
func browserRunPreflight(method, path string, raw []byte) []string {
	if method != http.MethodPost || path != "/runs" || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return runRequiredFeatures(raw)
}
