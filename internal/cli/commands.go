package cli

import (
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
)

// Root builds the full command tree.
func Root() *Command {
	root := &Command{
		Name:    "myco",
		Summary: "Messaging hub and CLI that connects AI agents (Claude Code, Codex, ...) across machines",
		Long: `Agents register with a hub, discover each other, and exchange tasks through
per-agent inboxes. A runner turns any agent CLI into an always-reachable worker.`,
		Examples: []string{
			"myco agent register --hub https://hub.example.com --name coder --capability code,github",
			"myco agent list --online",
			`myco message send --to coder --body "Fix the failing test in ./pkg/api" --wait 10m`,
			"myco runner start --harness claude --workdir ~/src/project",
			"myco schema --all",
		},
		Flags: []Flag{
			{Name: "format", Kind: Enum, Values: []string{"auto", "json", "text"}, Default: "auto", Env: "MYCO_FORMAT",
				Help: "Output format; auto = text on a TTY, json otherwise", Persistent: true},
			{Name: "config", Kind: String, Env: config.EnvConfig, Help: "Config file (default ~/.myco/config.yaml)", Persistent: true},
			{Name: "hub", Kind: String, Env: config.EnvHub, Help: "Hub URL (overrides config)", Persistent: true},
		},
	}
	root.Add(
		hubCommands(),
		agentCommands(),
		enrollmentCommands(),
		messageCommands(),
		broadcastCommands(),
		fileCommands(),
		inboxCommands(),
		threadCommands(),
		runnerCommands(),
		harnessCommands(),
		doctorCommand(),
		configCommands(),
		schemaCommand(root),
		versionCommand(),
	)
	return root
}
