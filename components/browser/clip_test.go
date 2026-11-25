package main

import (
	"encoding/json"
	"math"
	"testing"
)

func TestScreenshotClipRequiresCompleteFiniteExclusiveCoordinates(t *testing.T) {
	valid := `{"id":"shot","type":"screenshot","clip":{"x":0,"y":2.5,"width":30,"height":40}}`
	var value node
	if err := json.Unmarshal([]byte(valid), &value); err != nil || !value.Clip.valid() {
		t.Fatal("valid clip rejected", err)
	}
	for _, raw := range []string{
		`{"id":"shot","type":"screenshot","clip":null}`,
		`{"id":"shot","type":"screenshot","clip":{"width":30,"height":40}}`,
		`{"id":"shot","type":"screenshot","clip":{"x":0,"y":0,"width":0,"height":40}}`,
		`{"id":"shot","type":"screenshot","clip":{"x":-1,"y":0,"width":30,"height":40}}`,
		`{"id":"shot","type":"screenshot","clip":{"x":0,"y":0,"width":30,"height":40,"scale":2}}`,
		`{"id":"shot","type":"screenshot","full":false,"clip":{"x":0,"y":0,"width":30,"height":40}}`,
		`{"id":"shot","type":"screenshot","target":null,"clip":{"x":0,"y":0,"width":30,"height":40}}`,
		`{"id":"shot","type":"click","clip":{"x":0,"y":0,"width":30,"height":40}}`,
	} {
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			t.Fatal("malformed clip accepted", raw)
		}
	}
	for _, number := range []float64{math.NaN(), math.Inf(1), -1} {
		if (&screenshotClip{X: number, Width: 1, Height: 1}).valid() {
			t.Fatal("invalid coordinate", number)
		}
	}
	var graph script
	graph.Graph.Nodes = []node{{ID: "shot", Type: "screenshot", Clip: &screenshotClip{Width: 10, Height: 10}}}
	if _, err := linearOrder(graph, ""); err != nil {
		t.Fatal(err)
	}
	graph.Graph.Nodes[0].Target = &target{}
	if _, err := linearOrder(graph, ""); err == nil {
		t.Fatal("clip mixed with target")
	}
}
