package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
)

func TestPageRecordQueryCLIThroughHub(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	coder := newCLI(t, h, filepath.Join(home, "coder.yaml"))
	coder.must(c, "agent", "register", "--name", "coder")
	dir := filepath.Join(t.TempDir(), "issues")
	coder.must(c, "template", "init", "--dir", dir, "--name", "Issues", "--id", "coder/query")
	raw, err := os.ReadFile(filepath.Join(dir, "myco.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	json.Unmarshal(raw, &manifest)
	manifest["data"] = map[string]any{"issues": map[string]any{"fields": map[string]string{"n": "number", "title": "text", "status": "select", "assignee": "person", "created": "datetime"}}}
	raw, _ = json.Marshal(manifest)
	os.WriteFile(filepath.Join(dir, "myco.json"), raw, 0600)
	coder.must(c, "template", "publish", "--dir", dir)
	var used api.TemplateUsed
	json.Unmarshal([]byte(coder.must(c, "template", "use", "--template", "coder/query")), &used)
	page := used.Page.ID
	bodies := []string{
		`{"n":1,"title":"One","status":"progress","assignee":"alex","created":"2026-11-24T00:00:00Z"}`,
		`{"n":3,"title":"Three","status":"progress","assignee":"alex","created":"2026-11-25T00:00:00Z"}`,
		`{"n":2,"title":"Two","status":"todo","created":"2026-11-26T00:00:00Z"}`,
		`{"title":"Missing","status":"done"}`,
	}
	for i, body := range bodies {
		coder.must(c, "page", "data", "put", "--page", page, "--collection", "issues", "--id", fmt.Sprint("i", i), "--json", body)
	}
	list := func(extra ...string) api.PageRecords {
		t.Helper()
		args := append([]string{"page", "data", "list", "--page", page, "--collection", "issues"}, extra...)
		var r api.PageRecords
		if err := json.Unmarshal([]byte(coder.must(c, args...)), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	cases := []struct {
		flags []string
		ids   string
	}{
		{[]string{"--sort", "-n"}, "i1,i2,i0,i3"},
		{[]string{"--sort", "n"}, "i0,i2,i1,i3"},
		{[]string{"--where", "assignee:alex", "--where", "status:progress", "--sort", "-n"}, "i1,i0"},
		{[]string{"--where", "status:ne:progress"}, "i2,i3"},
		{[]string{"--where", "status:in:todo,done"}, "i2,i3"},
		{[]string{"--where", "n:exists:false"}, "i3"},
		{[]string{"--where", "n:exists"}, "i0,i1,i2"},
		{[]string{"--where", "n:gt:2"}, "i1"},
		{[]string{"--where", "n:gte:2", "--sort", "n"}, "i2,i1"},
		{[]string{"--where", "n:lt:2"}, "i0"},
		{[]string{"--where", "n:lte:2", "--sort", "n"}, "i0,i2"},
		{[]string{"--where", "created:>=:2026-11-25", "--where", "created:<:2026-11-26"}, "i1"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.flags, " "), func(t *testing.T) {
			r := list(tc.flags...)
			ids := []string{}
			for _, rec := range r.Records {
				ids = append(ids, rec.ID)
			}
			if strings.Join(ids, ",") != tc.ids || r.QueryVersion != 1 {
				t.Fatalf("%+v", r)
			}
		})
	}
	first := list("--fields", "n,title", "--sort", "-n", "--limit", "1")
	if len(first.Records) != 1 || first.Records[0].ID != "i1" || first.Records[0].Version != 1 || first.Next == "" || strings.Contains(string(first.Records[0].Data), "assignee") {
		t.Fatalf("projection: %+v", first)
	}
	second := list("--fields", "n,title", "--sort", "-n", "--limit", "2", "--after", first.Next)
	if len(second.Records) != 2 || second.Records[0].ID != "i2" || second.Records[1].ID != "i0" {
		t.Fatalf("cursor: %+v", second)
	}
	rawCount := coder.must(c, "page", "data", "list", "--page", page, "--collection", "issues", "--where", "status:progress", "--count", "--limit", "1")
	var count api.PageRecordCount
	json.Unmarshal([]byte(rawCount), &count)
	if count.Count != 2 || strings.Contains(rawCount, "records") {
		t.Fatalf("count: %s", rawCount)
	}
	for _, flags := range [][]string{{"--fields", "n);DROP_TABLE"}, {"--where", "n.x:1"}, {"--sort", "-n;"}, {"--limit", "1001"}, {"--after", "broken"}, {"--sort", "n", "--after", first.Next}} {
		args := append([]string{"page", "data", "list", "--page", page, "--collection", "issues"}, flags...)
		code, _, errb := coder.run(c, args...)
		if code == 0 || !strings.Contains(errb, "invalid_argument") {
			t.Fatalf("accepted invalid query %v: %d %s", flags, code, errb)
		}
	}
	coder.must(c, "page", "data", "put", "--page", page, "--collection", "issues", "--id", "new", "--json", `{"n":4}`)
	code, _, errb := coder.run(c, "page", "data", "list", "--page", page, "--collection", "issues", "--fields", "n,title", "--sort", "-n", "--after", first.Next)
	if code == 0 || !strings.Contains(errb, "conflict") {
		t.Fatalf("stale cursor: %d %s", code, errb)
	}
	// Bypass client validation: the actual HTTP boundary must reject unsafe
	// fields and malformed options, and still require authentication for count.
	token := h.register(api.RegisterRequest{Name: "reader"})
	for _, query := range []url.Values{
		{"fields": {"n');DROP TABLE page_records--"}},
		{"where": {"n.x:1"}}, {"limit": {"banana"}}, {"count": {"maybe"}},
		{"sort": {"-n;"}}, {"after": {"malformed"}},
	} {
		req, _ := http.NewRequestWithContext(c, "GET", h.url+"/v1/pages/"+page+"/data?"+query.Encode(), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("raw query accepted: %v, status %d", query, res.StatusCode)
		}
	}
	req, _ := http.NewRequestWithContext(c, "GET", h.url+"/v1/pages/"+page+"/data?count=true", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatal("count bypassed authentication", res.StatusCode)
	}
	if len(list().Records) != 5 {
		t.Fatal("legacy list changed")
	}
}
