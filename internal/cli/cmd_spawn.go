package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/harness/machinenode"
)

func agentSpawnCommand() *Command {
	return &Command{
		Name:     "spawn",
		Summary:  "Start a new agent session in a directory on another machine and join it",
		Mutating: true,
		Long: `Asks a machine's machine node (created automatically when the machine joined with a link) to start
a new Claude Code, Codex or other agent CLI session in a directory there, enroll it as an agent and
have that machine's runner serve it. Prints the new agent's name; send it work with message send.

The machine decides, locally: who may ask (the machine node's allow_from, inherited from the join
link; change it with agent update), which directories (spawn.dirs in its ~/.myco/machine.yaml,
default: its home directory) and the highest access (spawn.max_access, default workspace).`,
		Examples: []string{
			`myco agent spawn --on wsl --dir ~/code/app --description "reviews the web app"`,
			`myco agent spawn --on mac --dir ~/src/api --cli codex --name api-coder --access workspace`,
			`myco agent spawn --on wsl --dir ~/code/app --prompt "Read README.md and summarise the architecture first"`,
		},
		Flags: []Flag{
			{Name: "on", Kind: String, Required: true, Help: "Machine name (as in its machine=<name> label) or the machine node's agent name"},
			{Name: "dir", Kind: String, Required: true, Help: "Directory on that machine (~/... is that machine's home)"},
			{Name: "cli", Kind: Enum, Values: machinenode.SpawnableCLIs, Default: "claude", Help: "Agent CLI to start"},
			{Name: "name", Kind: String, Help: "Agent name (the machine's prefix is added; default: <prefix><dir>-<cli>)"},
			{Name: "display-name", Kind: String, Help: "Human-friendly name"},
			{Name: "description", Kind: String, Help: "What the new agent does (also told to the session)"},
			{Name: "label", Kind: List, Help: "Extra labels, e.g. group=web"},
			{Name: "access", Kind: Enum, Values: []string{"readonly", "workspace", "full"}, Default: "workspace", Help: "Permission level (capped by the machine)"},
			{Name: "model", Kind: String, Help: "Model, in the CLI's own naming"},
			{Name: "prompt", Kind: String, Help: "First instructions for the new session"},
			{Name: "timeout", Kind: Duration, Default: "5m", Help: "How long to wait for the machine to answer"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("on", "dir"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			on := strings.TrimSpace(e.in.String("on"))
			node, err := findMachineNode(e, on)
			if err != nil {
				return err
			}
			req := api.SpawnRequest{CLI: e.in.String("cli"), Dir: remoteDir(e.in.String("dir")), Name: e.in.String("name"),
				DisplayName: e.in.String("display-name"), Description: e.in.String("description"), Labels: e.in.List("label"),
				Access: e.in.String("access"), Model: e.in.String("model"), Prompt: e.in.String("prompt")}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "spawn", "machine_node": node.Name, "node_online": node.Online,
					"allow_from": node.AllowFrom, "request": req})
			}
			body, err := json.Marshal(req)
			if err != nil {
				return err
			}
			sent, err := c.Send(e.in.Ctx, api.SendRequest{To: node.Name, Type: "task", Body: string(body)}, false)
			if err != nil {
				return err
			}
			wait := e.in.Duration("timeout")
			reply, err := c.WaitReply(e.in.Ctx, sent.Message.ID, wait)
			if err != nil {
				return err
			}
			if reply == nil {
				hint := ""
				if !node.Online {
					hint = " (the machine is offline: is its runner running?)"
				}
				return &Error{Code: "timeout", Message: fmt.Sprintf("no answer from %s within %s%s", node.Name, wait, hint),
					Suggestion: "keep waiting with: myco message wait --id " + sent.Message.ID + " --timeout 10m",
					Retryable:  true, Exit: ExitTimeout}
			}
			var res api.SpawnResult
			if reply.Type != "reply" || json.Unmarshal([]byte(reply.Body), &res) != nil {
				return &Error{Code: "spawn_failed", Message: fmt.Sprintf("%s did not return a spawn result: %s", node.Name, truncate(reply.Body, 500)),
					Suggestion: "upgrade myco on that machine and restart its runner: myco runner service restart", Exit: ExitError}
			}
			switch {
			case res.Denied:
				return &Error{Code: "permission_denied", Message: res.Error, Exit: ExitPermission,
					Suggestion: "the machine's own rules decide (allow_from of " + node.Name + ", spawn.dirs and spawn.max_access in its ~/.myco/machine.yaml)"}
			case res.Error != "":
				return &Error{Code: "spawn_failed", Message: res.Error, Exit: ExitError,
					Suggestion: "check that the CLI works on that machine (myco doctor --all there) and see its runner logs (myco runner service logs)"}
			}
			return e.p.Print(map[string]any{"message_id": sent.Message.ID, "machine_node": node.Name, "result": res}, func(w io.Writer) {
				fmt.Fprintf(w, "spawned %s on %s (%s, access %s) in %s\n", res.Agent, on, res.CLI, res.Access, res.Workdir)
				fmt.Fprintf(w, "session: %s\n", res.SessionID)
				if res.Reply != "" {
					fmt.Fprintf(w, "first reply: %s\n", truncate(res.Reply, 300))
				}
				fmt.Fprintf(w, "send it work: myco message send --to %s --body \"...\"\n", res.Agent)
			})
		}),
	}
}

// remoteDir undoes the local shell's ~ expansion: --dir ~/code/app arrives
// as /home/me/code/app, but means the other machine's home.
func remoteDir(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(dir, home+"/"); ok {
		return "~/" + rest
	}
	return dir
}

// findMachineNode resolves --on: the machine node's own name, or a machine
// name matched through the machine=<name> label.
func findMachineNode(e *env, on string) (*api.Agent, error) {
	c, err := e.agentClient()
	if err != nil {
		return nil, err
	}
	if a, err := c.GetAgent(e.in.Ctx, on); err == nil && a.Binding.Harness == machinenode.Name {
		return a, nil
	}
	agents, err := c.ListAgents(e.in.Ctx, "", false, "machine="+on+",harness="+machinenode.Name)
	if err != nil {
		return nil, err
	}
	switch len(agents) {
	case 1:
		return &agents[0], nil
	case 0:
		return nil, errorf("not_found", ExitNotFound,
			"the machine must have joined with a link and run myco with agent spawn support: on it, upgrade (install-client.sh) and restart the runner (myco runner service restart); list machine nodes with: myco agent list --selector harness=machine",
			"no machine node for %q", on)
	}
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return nil, errorf("conflict", ExitConflict, "pass the machine node's name with --on: "+strings.Join(names, ", "),
		"several machine nodes match %q", on)
}
