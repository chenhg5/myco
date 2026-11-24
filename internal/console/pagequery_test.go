package console_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/client"
)

func TestConsolePageQueryProxy(t *testing.T) {
	r := newRig(t)
	tok := r.consoleToken("queries")
	code, hub, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Queries", "url": r.hubURL, "token": tok})
	if code != 201 {
		t.Fatal(hub)
	}
	owner, _ := client.New(r.hubURL, tok)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("index.html")
	w.Write([]byte("<p>query</p>"))
	z.Close()
	page, err := owner.PublishPage(context.Background(), client.PageInput{Name: "Query"}, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"n":1,"title":"One"}`, `{"n":2,"title":"Two"}`} {
		if _, err := owner.PutPageRecord(context.Background(), page.ID, "issues", "", json.RawMessage(body), nil); err != nil {
			t.Fatal(err)
		}
	}
	path := "/app/hubs/" + hub["id"].(string) + "/pages/" + page.ID + "/data?collection=issues&query_version=1"
	code, out, raw := r.do("GET", path+"&fields=n&sort=-n&limit=1", nil)
	if code != 200 || out["next"] == nil || strings.Contains(string(raw), "title") || !strings.Contains(string(raw), `"n":2`) {
		t.Fatalf("projection: %d %s", code, raw)
	}
	code, out, raw = r.do("GET", path+"&count=true&where=n:gte:2", nil)
	if code != 200 || out["count"] != float64(1) || strings.Contains(string(raw), "records") {
		t.Fatalf("count: %d %s", code, raw)
	}
	code, _, _ = r.do("GET", path+"&fields=n.x", nil)
	if code != 400 {
		t.Fatal("invalid field", code)
	}
}
