package machinenode

import (
	"context"
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
)

const ManagedForkCLI = "myco-fork"
const ManagedRetireCLI = "myco-retire"

func (n *Node) CanManageForks() bool { return n.Deps.ManagedFork != nil }
func (n *Node) managedFork(ctx context.Context, req harness.Request, id string, retire bool) api.SpawnResult {
	if id == "" || len(id) > 100 {
		return denied("a valid managed fork ID is required")
	}
	if n.Deps.ManagedFork == nil {
		return denied("unsupported: upgrade the machine runner and use runner --all for managed fork/retire")
	}
	out, err := n.Deps.ManagedFork(ctx, req, id, retire)
	if err != nil {
		return api.SpawnResult{Error: err.Error()}
	}
	return out
}
