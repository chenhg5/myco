package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestTaskTreeTextBranchesAndReferences(t *testing.T) {
	n := api.TaskTreeNode{ID: "root", Agent: "lead", Status: "held", AgeSeconds: 20, WaitingOn: []api.TaskTreeWait{{Kind: "task", ID: "child"}}, Children: []api.TaskTreeNode{
		{ID: "child", Agent: "worker", Status: "queued", AgeSeconds: 10, Children: []api.TaskTreeNode{{ID: "leaf", Agent: "helper", Status: "done", AgeSeconds: 2}}},
		{ID: "child", Agent: "worker", Status: "queued", Relation: "thread_inferred", Reference: true},
	}}
	var b bytes.Buffer
	printTaskTree(&b, n, "", true)
	for _, want := range []string{"root lead held age:20s waits:task=child", "├─ child", "│  └─ leaf", "└─ child", "(reference)", "(thread inferred)"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %q in tree:\n%s", want, b.String())
		}
	}
}
