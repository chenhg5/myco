package main

import (
	"encoding/json"
	"testing"
)

func TestAppLinearContract(t *testing.T) {
	var in runOptions
	raw := `{"script":{"name":"Form","params":[{"name":"password","type":"secret"}],"graph":{"nodes":[{"id":"c","type":"confirm","message":"Before submit","pos":[0,256]},{"id":"a","type":"navigate","url":"https://example.com","pos":[0,0]},{"id":"b","type":"type","sensitive":true,"value":"${password}","target":{"locators":[{"kind":"role","role":"textbox","name":"Password"},{"kind":"css","value":"#password"}],"frame":["iframe"],"shadow":["login-form"]},"pos":[0,128]}],"edges":[["a","b"],["b","c"]]},"on_error":"stop"},"script_id":"scr_a","script_version":7,"profile":"p_test","params":{},"secrets":{"password":"private"},"start_at":"b","breakpoints":["c"]}`
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	seq, err := linearOrder(in.Script, in.StartAt)
	if err != nil || len(seq) != 2 || seq[0].ID != "b" || seq[1].Type != "confirm" {
		t.Fatalf("sequence: %+v %v", seq, err)
	}
	if seq[0].Target.Frame[0] != "iframe" || len(seq[0].Target.Locators) != 2 {
		t.Fatal("locator chain lost")
	}
	for _, change := range []func(){func() { in.Script.Graph.Nodes[0].Type = "loop" }, func() {
		in.Script.Graph.Nodes[0].Type = "confirm"
		in.Script.Graph.Edges = append(in.Script.Graph.Edges, []string{"c", "a"})
	}, func() { in.Script.Graph.Edges = in.Script.Graph.Edges[:2]; in.Script.Graph.Nodes[2].Value = "private" }} {
		change()
		if _, err = linearOrder(in.Script, ""); err == nil {
			t.Fatal("invalid/unsupported script accepted")
		}
	}
}

func TestExplicitSideEffectContract(t *testing.T) {
	s := script{OnError: "stop"}
	s.Graph.Nodes = []node{{ID: "a", Type: "click"}, {ID: "b", Type: "dialog", Dialog: "dismiss"}, {ID: "c", Type: "wait", Wait: "url", Value: "/done"}, {ID: "d", Type: "tab", Tab: "popup"}, {ID: "e", Type: "wait", Wait: "download"}, {ID: "f", Type: "tab", Tab: "0"}}
	for i := 1; i < len(s.Graph.Nodes); i++ {
		s.Graph.Edges = append(s.Graph.Edges, []string{s.Graph.Nodes[i-1].ID, s.Graph.Nodes[i].ID})
	}
	if _, err := linearOrder(s, ""); err != nil {
		t.Fatal(err)
	}
	for _, tab := range []string{"-1", "100", "01", "popup:1", "https://example.com"} {
		s.Graph.Nodes[3].Tab = tab
		if _, err := linearOrder(s, ""); err == nil {
			t.Fatalf("invalid tab %s", tab)
		}
	}
	s.Graph.Nodes[3].Tab = "opener"
	s.Graph.Nodes[1].Dialog = "Enter"
	if _, err := linearOrder(s, ""); err == nil {
		t.Fatal("keyboard key accepted as dialog action")
	}
}
