package browserautomation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// ErrUnsupportedNode means capability negotiation rejected the run before its POST.
var ErrUnsupportedNode = errors.New("unsupported_node")

// Old protocol-1 engines ignore unknown node fields. Detect clip even when it
// is null/malformed: an older engine must never turn that node into a full-page
// screenshot. Check main and child script snapshots before sending any run.
func runNeedsScreenshotClip(raw []byte) bool {
	var envelope struct {
		Script  json.RawMessage            `json:"script"`
		Scripts map[string]json.RawMessage `json:"scripts"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return false
	}
	scripts := []json.RawMessage{envelope.Script}
	for _, script := range envelope.Scripts {
		scripts = append(scripts, script)
	}
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
			if _, present := node["clip"]; present {
				return true
			}
		}
	}
	return false
}

func requireScreenshotClip(ctx context.Context, hc *http.Client, address, secret, scope string) error {
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
	for _, feature := range status.Features {
		if feature == "screenshot_clip" {
			return nil
		}
	}
	return ErrUnsupportedNode
}

// Keep the preflight tied to the very same process endpoint/credential as the
// run POST; checking c.Call recursively could race a component replacement.
func needsClipPreflight(method, path string, raw []byte) bool {
	return method == http.MethodPost && path == "/runs" && len(bytes.TrimSpace(raw)) > 0 && runNeedsScreenshotClip(raw)
}
