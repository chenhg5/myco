package client

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/chenhg5/myco/internal/api"
)

// ListProjects lists the projects the caller may see; mine keeps those it
// takes part in.
func (c *Client) ListProjects(ctx context.Context, mine bool) ([]api.Project, error) {
	q := url.Values{}
	if mine {
		q.Set("mine", "true")
	}
	var out api.ProjectList
	return out.Projects, c.do(ctx, http.MethodGet, "/v1/projects", q, nil, &out)
}

// CreateProject starts a project (people only).
func (c *Client) CreateProject(ctx context.Context, req api.CreateProjectRequest) (*api.Project, error) {
	var out api.Project
	return &out, c.do(ctx, http.MethodPost, "/v1/projects", nil, req, &out)
}

// GetProject reads a project; bodies includes its documents' text.
func (c *Client) GetProject(ctx context.Context, name string, bodies bool) (*api.Project, error) {
	q := url.Values{}
	if bodies {
		q.Set("docs", "body")
	}
	var out api.Project
	return &out, c.do(ctx, http.MethodGet, "/v1/projects/"+esc(name), q, nil, &out)
}

// UpdateProject changes a project.
func (c *Client) UpdateProject(ctx context.Context, name string, req api.UpdateProjectRequest) (*api.Project, error) {
	var out api.Project
	return &out, c.do(ctx, http.MethodPatch, "/v1/projects/"+esc(name), nil, req, &out)
}

// DeleteProject removes a project with its members and documents.
func (c *Client) DeleteProject(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/projects/"+esc(name), nil, nil, nil)
}

// PutProjectMember adds a node to a project, or changes its role or note.
func (c *Client) PutProjectMember(ctx context.Context, project, agent string, req api.ProjectMemberRequest) (*api.Project, error) {
	var out api.Project
	return &out, c.do(ctx, http.MethodPut, "/v1/projects/"+esc(project)+"/members/"+esc(agent), nil, req, &out)
}

// RemoveProjectMember takes a node out of a project.
func (c *Client) RemoveProjectMember(ctx context.Context, project, agent string) (*api.Project, error) {
	var out api.Project
	return &out, c.do(ctx, http.MethodDelete, "/v1/projects/"+esc(project)+"/members/"+esc(agent), nil, nil, &out)
}

// GetProjectDoc reads a shared document.
func (c *Client) GetProjectDoc(ctx context.Context, project, doc string) (*api.ProjectDoc, error) {
	var out api.ProjectDoc
	return &out, c.do(ctx, http.MethodGet, "/v1/projects/"+esc(project)+"/docs/"+esc(doc), nil, nil, &out)
}

// PutProjectDoc writes a shared document.
func (c *Client) PutProjectDoc(ctx context.Context, project, doc, body string) (*api.ProjectDoc, error) {
	var out api.ProjectDoc
	return &out, c.do(ctx, http.MethodPut, "/v1/projects/"+esc(project)+"/docs/"+esc(doc), nil, api.ProjectDocRequest{Body: body}, &out)
}

// DeleteProjectDoc removes a shared document.
func (c *Client) DeleteProjectDoc(ctx context.Context, project, doc string) error {
	return c.do(ctx, http.MethodDelete, "/v1/projects/"+esc(project)+"/docs/"+esc(doc), nil, nil, nil)
}

// PutProjectFile uploads a file into a project (any kind, up to 20 MB);
// contentType "" lets the hub guess it from the name.
func (c *Client) PutProjectFile(ctx context.Context, project, name, contentType string, body io.Reader, size int64) (*api.ProjectDoc, error) {
	u := c.urlFor("/v1/projects/" + esc(project) + "/files/" + esc(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), body)
	if err != nil {
		return nil, err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	var out api.ProjectDoc
	return &out, c.send(ctx, req, &out)
}

// OpenProjectFile streams a project file's content; the caller closes it.
func (c *Client) OpenProjectFile(ctx context.Context, project, name string) (io.ReadCloser, string, error) {
	u := c.urlFor("/v1/projects/" + esc(project) + "/files/" + esc(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, "", decodeError(resp.StatusCode, data)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}
