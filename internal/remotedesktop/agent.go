package remotedesktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// Agent actions through the component (components/remote/agent.go): a still
// frame of the screen, and validated input for a frame. The App calls these
// only for a job whose grant it checked; the component never sees the Hub.

// AgentActions reports whether a component version offers agent actions.
func AgentActions(version string) bool {
	return version != "" && version != "0.1.0" && version != "0.2.0"
}

// Observation is one still frame: Width and Height are the image's, which
// input's normalized coordinates refer to.
type Observation struct {
	FrameID    string    `json:"frame_id"`
	CapturedAt time.Time `json:"captured_at"`
	DisplayID  string    `json:"display_id"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	MediaType  string    `json:"media_type"`
	Image      []byte    `json:"image"`
}

// ComponentError is the component's refusal: Code (invalid_argument,
// stale_frame, control_not_allowed, input_failed, screen_capture_failed…)
// and, for input, how many events it applied first.
type ComponentError struct {
	Code    string
	Applied *int
}

func (e *ComponentError) Error() string { return e.Code }

// Observe captures a frame at most maxWidth wide (0: 1280), "jpeg" or "png".
func (c *Component) Observe(ctx context.Context, maxWidth int, format string) (*Observation, error) {
	var out Observation
	if err := c.agentCall(ctx, "/agent/observe", map[string]any{"max_width": maxWidth, "format": format}, 12<<20, 25*time.Second, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Input applies events (normalized to frameID's image) in order and
// returns how many it applied. Cancelling ctx stops the rest; the component
// releases every key and button either way.
func (c *Component) Input(ctx context.Context, frameID string, events json.RawMessage) (int, error) {
	var out struct {
		Applied int `json:"applied"`
	}
	if err := c.agentCall(ctx, "/agent/input", map[string]any{"frame_id": frameID, "events": events}, 4096, 30*time.Second, &out); err != nil {
		return 0, err
	}
	return out.Applied, nil
}

// AgentStatus is what a running component says it offers agents.
func (c *Component) AgentStatus(ctx context.Context) (actions []string, control bool, err error) {
	var st struct {
		Actions []string `json:"agent_actions"`
		Control bool     `json:"control"`
	}
	err = c.Call(ctx, "GET", "/status", nil, &st)
	return st.Actions, st.Control, err
}

func (c *Component) agentCall(ctx context.Context, path string, in any, limit int64, timeout time.Duration, out any) error {
	c.mu.Lock()
	address, secret := c.address, c.secret
	c.mu.Unlock()
	if address == "" {
		return errors.New("component_not_running")
	}
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+address+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	hc := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("component redirect refused") }}
	defer hc.CloseIdleConnections()
	res, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("component_unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		var v struct {
			Error   string `json:"error"`
			Applied *int   `json:"applied"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&v)
		if v.Error == "" {
			v.Error = "component_failed"
		}
		return &ComponentError{Code: v.Error, Applied: v.Applied}
	}
	return json.NewDecoder(io.LimitReader(res.Body, limit)).Decode(out)
}
