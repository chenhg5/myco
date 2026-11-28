package httpapi_test

import (
	"testing"

	"github.com/myco-ai/myco/internal/app"
)

func TestWorkflowRunViewsHTTPValidationAndScope(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	h.register("worker", nil)
	definition := map[string]any{"name": "demo-view", "steps": []any{map[string]any{"id": "work", "agent": "worker", "prompt": "rendered-private-task"}}, "edges": []any{map[string]any{"from": "work", "to": "$end"}}}
	h.do("POST", "/v1/workflows", adminToken, map[string]any{"definition": definition}).want(t, 201)
	started := h.do("POST", "/v1/workflows/demo-view/runs", adminToken, map[string]any{}).want(t, 201)
	id := started.body["id"].(string)
	instance := started.body["steps"].([]any)[0].(map[string]any)["id"].(string)
	path := "/v1/workflow-runs/" + id
	compact := h.do("GET", path+"?view=compact", adminToken, nil).want(t, 200)
	if compact.body["view"] != "compact" {
		t.Fatal(string(compact.raw))
	}
	one := h.do("GET", path+"?view=step&step="+instance, adminToken, nil).want(t, 200)
	rows := one.body["steps"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != instance {
		t.Fatal(string(one.raw))
	}
	for _, query := range []string{"?view=unknown", "?view=", "?view=step"} {
		h.do("GET", path+query, adminToken, nil).wantErr(t, 400, "invalid_argument")
	}
	h.do("GET", path+"?view=step&step=missing", adminToken, nil).wantErr(t, 404, "not_found")
	other := h.do("POST", "/v1/workflows/demo-view/runs", adminToken, map[string]any{}).want(t, 201)
	foreign := other.body["steps"].([]any)[0].(map[string]any)["id"].(string)
	h.do("GET", path+"?view=step&step="+foreign, adminToken, nil).wantErr(t, 404, "not_found")
	h.do("GET", path+"?view=compact", "", nil).wantErr(t, 401, "unauthenticated")
	outsider := h.register("outsider", nil)
	h.do("POST", "/v1/members", adminToken, map[string]any{"name": "bob"}).want(t, 201)
	h.do("PUT", "/v1/agents/outsider/owner", adminToken, map[string]any{"owner": "bob"}).want(t, 200)
	for _, query := range []string{"", "?view=compact", "?view=step&step=" + instance} {
		h.do("GET", path+query, outsider, nil).wantErr(t, 404, "not_found")
	}
}
