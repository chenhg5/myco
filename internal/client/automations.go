package client

import (
	"context"
	"net/http"

	"github.com/chenhg5/myco/internal/api"
)

// Automations lists the automations the caller may see (a console token:
// its own, or all for those who manage the hub).
func (c *Client) Automations(ctx context.Context) ([]api.Automation, error) {
	var out api.AutomationList
	if err := c.do(ctx, http.MethodGet, "/v1/console/automations", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Automations, nil
}

// CreateAutomation makes one owned by the caller; a webhook's secret is in
// the answer, once.
func (c *Client) CreateAutomation(ctx context.Context, in api.AutomationRequest) (*api.AutomationResponse, error) {
	var out api.AutomationResponse
	if err := c.do(ctx, http.MethodPost, "/v1/console/automations", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Automation returns one with its newest runs.
func (c *Client) Automation(ctx context.Context, id string) (*api.AutomationDetail, error) {
	var out api.AutomationDetail
	if err := c.do(ctx, http.MethodGet, "/v1/console/automations/"+esc(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AutomationRun is one of an automation's runs, with what its webhook call
// carried (hubs from v0.20.18; older ones: not found).
func (c *Client) AutomationRun(ctx context.Context, id, run string) (*api.AutomationRun, error) {
	var out api.AutomationRun
	if err := c.do(ctx, http.MethodGet, "/v1/console/automations/"+esc(id)+"/runs/"+esc(run), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateAutomation changes what in sets.
func (c *Client) UpdateAutomation(ctx context.Context, id string, in api.AutomationRequest) (*api.AutomationResponse, error) {
	var out api.AutomationResponse
	if err := c.do(ctx, http.MethodPatch, "/v1/console/automations/"+esc(id), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAutomation removes one and its runs.
func (c *Client) DeleteAutomation(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/console/automations/"+esc(id), nil, nil, nil)
}

// RunAutomation runs one now (also when it is turned off).
func (c *Client) RunAutomation(ctx context.Context, id string) (*api.AutomationRun, error) {
	var out api.AutomationRun
	if err := c.do(ctx, http.MethodPost, "/v1/console/automations/"+esc(id)+"/run", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
