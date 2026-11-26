package api

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestBrowserReplayPreflightsEntireRecordedGraph(t *testing.T) {
	good := `{"nodes":[{"id":"n1","type":"navigate","url":"https://example.com/","pos":[0,0],"title":"Open","timeout_ms":3000,"on_error":"retry:2"},{"id":"n2","type":"click","target":{"locators":[{"kind":"xpath","value":"//button[@id='go']"},{"kind":"css","value":"#go"}]}},{"id":"n3","type":"extract","what":"text","into":"title","target":{"locators":[{"kind":"role","role":"heading","name":"Documentation"}]} }],"edges":[["n1","n2"],["n2","n3"]]}`
	if err := ValidateBrowserReplayGraph(json.RawMessage(good)); err != nil {
		t.Fatal(err)
	}
	for _, replace := range [][2]string{
		{`"on_error":"retry:2"`, `"on_error":"goto:n3"`},
		{`"type":"click"`, `"type":"js"`},
		{`"what":"text"`, `"what":"js"`},
		{`"kind":"xpath"`, `"kind":"js"`},
		{`"type":"click"`, `"type":"click","code":""`},
		{`"title":"Open"`, `"title":"${value}"`},
		{`"kind":"css","value":"#go"`, `"kind":"css","value":"${selector}"`},
		{`"on_error":"retry:2"`, `"on_error":"retry:6"`},
		{`["n2","n3"]`, `["n1","n3"]`},
		{`["n2","n3"]`, `["n2","missing"]`},
		{`["n2","n3"]`, `["n2","n1"]`},
	} {
		raw := strings.Replace(good, replace[0], replace[1], 1)
		if err := ValidateBrowserReplayGraph(json.RawMessage(raw)); err == nil {
			t.Fatal("unsafe graph accepted", replace)
		}
	}
	var failure *BrowserReplayValidationError
	if !errors.As(ValidateBrowserReplayGraph(json.RawMessage(strings.Replace(good, `"title":"Open"`, `"title":"${value}"`, 1))), &failure) || failure.Node != "n1" {
		t.Fatal("missing safe preflight node", failure)
	}
}
func TestBrowserReplayWhitelistAndLimits(t *testing.T) {
	graph := func(node string) json.RawMessage { return json.RawMessage(`{"nodes":[` + node + `],"edges":[]}`) }
	for _, node := range []string{
		`{"id":"a","type":"note","note":"Public note"}`,
		`{"id":"a","type":"wait","wait":"time","ms":60000}`,
		`{"id":"a","type":"wait","wait":"network"}`,
		`{"id":"a","type":"screenshot","clip":{"x":0,"y":0,"width":200,"height":100},"into":"image"}`,
		`{"id":"a","type":"assert","check":"hidden","target":{"locators":[{"kind":"css","value":"#dialog"}]}}`,
		`{"id":"a","type":"extract","what":"table","target":{"locators":[{"kind":"testid","value":"table"}]}}`,
		`{"id":"a","type":"tab","tab":"new","url":"https://example.com/"}`,
	} {
		if err := ValidateBrowserReplayGraph(graph(node)); err != nil {
			t.Fatal(node, err)
		}
	}
	for _, node := range []string{
		`{"id":"a","type":"upload"}`, `{"id":"a","type":"confirm"}`, `{"id":"a","type":"branch"}`, `{"id":"a","type":"loop"}`, `{"id":"a","type":"subflow"}`,
		`{"id":"a","type":"wait","wait":"download"}`,
		`{"id":"a","type":"wait","wait":"time","ms":60001}`,
		`{"id":"a","type":"navigate","url":"file:///private/demo"}`,
		`{"id":"a","type":"navigate","url":"https://user:pass@example.com/"}`,
		`{"id":"a","type":"assert","check":"expr","code":"true"}`,
		`{"id":"a","type":"screenshot","clip":{"width":200,"height":100}}`,
		`{"id":"a","type":"type","sensitive":true,"value":"demo","target":{"locators":[{"kind":"css","value":"#password"}]}}`,
		`{"id":"a","type":"note","note":"` + strings.Repeat("x", BrowserReplayGraphMax) + `"}`,
	} {
		if err := ValidateBrowserReplayGraph(graph(node)); err == nil {
			t.Fatal("accepted refused graph", node[:min(100, len(node))])
		}
	}
	raw := json.RawMessage(`{"nodes":[{"id":"a","type":"note"},{"id":"b","type":"note"}],"edges":[]}`)
	if ValidateBrowserReplayGraph(raw) == nil {
		t.Fatal("disconnected nodes accepted")
	}
}

// Both module boundaries consume these public contract fixtures independently.
func TestBrowserReplaySharedContract(t *testing.T) {
	raw, err := os.ReadFile("../../components/browser/testdata/agent-replay-graphs.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name  string
		Valid bool
		Graph json.RawMessage
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			err := ValidateBrowserReplayGraph(fixture.Graph)
			if (err == nil) != fixture.Valid {
				t.Fatalf("valid=%t error=%v", fixture.Valid, err)
			}
		})
	}
}
