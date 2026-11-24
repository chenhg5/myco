package client

import (
	"context"
	"net/http"

	"github.com/myco-ai/myco/internal/api"
)

func (c *Client) ProjectAutomations(ctx context.Context, project string) ([]api.Automation, error) {
	var out api.AutomationList
	e := c.do(ctx, http.MethodGet, "/v1/projects/"+esc(project)+"/automations", nil, nil, &out)
	return out.Automations, e
}
func (c *Client) ChangeProjectAutomation(ctx context.Context, project, action, id string, in api.AutomationRequest) (*api.ProjectAutomationResponse, error) {
	method, path := http.MethodPost, "/v1/projects/"+esc(project)+"/automations"
	if action != "create" {
		path += "/" + esc(id)
		method = http.MethodPatch
	}
	var body any = in
	if action == "delete" {
		method = http.MethodDelete
		body = nil
	}
	var out api.ProjectAutomationResponse
	e := c.do(ctx, method, path, nil, body, &out)
	return &out, e
}
func (c *Client) SetProjectCoordinatorAutomations(ctx context.Context, project string, allow bool) (*api.Project, error) {
	var out api.Project
	e := c.do(ctx, http.MethodPut, "/v1/projects/"+esc(project)+"/automation-permission", nil, map[string]bool{"allowed": allow}, &out)
	return &out, e
}
