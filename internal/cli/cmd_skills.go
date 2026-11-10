package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

// agentSkillsCommands: what an agent can be asked to do (myco agent skills).
func agentSkillsCommands() *Command {
	target := []Flag{
		{Name: "name", Kind: String, Help: "Another agent (its owner, an admin, or a coordinator agent of the same owner); default: yourself"},
		{Name: "admin-token-file", Kind: String, Help: "File containing the admin token"},
	}
	clientFor := func(e *env) (*client.Client, error) {
		if e.in.String("name") != "" {
			return e.cardClient()
		}
		return e.agentClient()
	}
	current := func(e *env, c *client.Client) (*api.Agent, error) {
		if n := e.in.String("name"); n != "" {
			return c.GetAgent(e.in.Ctx, n)
		}
		return c.Me(e.in.Ctx)
	}
	print := func(e *env, a *api.Agent) error {
		return e.p.Print(a, func(w io.Writer) { writeSkills(w, a) })
	}
	save := func(e *env, c *client.Client, action string, skills []api.Skill) error {
		if e.in.Bool("dry-run") {
			return e.dryRun(map[string]any{"action": action, "agent": orDefault(e.in.String("name"), e.cfg.Agent), "skills": skills})
		}
		a, err := c.SetSkills(e.in.Ctx, e.in.String("name"), skills)
		if err != nil {
			return err
		}
		return print(e, a)
	}

	list := &Command{
		Name: "list", Summary: "Show an agent's skills",
		Examples: []string{"myco agent skills list", "myco agent skills list --name ios-claude --format json"},
		Flags:    target,
		Run: handler(func(e *env) error {
			c, err := clientFor(e)
			if err != nil {
				return err
			}
			a, err := current(e, c)
			if err != nil {
				return err
			}
			return print(e, a)
		}),
	}
	add := &Command{
		Name: "add", Summary: "Add a skill, or replace the one with the same name", Mutating: true,
		Examples: []string{
			`myco agent skills add --skill ios-release --summary "Build, sign and upload iOS test builds" --example "Ship main to TestFlight"`,
		},
		Flags: append([]Flag{
			{Name: "skill", Kind: String, Required: true, Help: "Short lowercase name, e.g. ios-release"},
			{Name: "summary", Kind: String, Help: "One line: what this skill covers"},
			{Name: "example", Kind: List, Help: "A request it fits (repeatable, at most 3)"},
		}, target...),
		Run: handler(func(e *env) error {
			if err := e.in.Require("skill"); err != nil {
				return err
			}
			c, err := clientFor(e)
			if err != nil {
				return err
			}
			a, err := current(e, c)
			if err != nil {
				return err
			}
			s := api.Skill{Name: strings.ToLower(strings.TrimSpace(e.in.String("skill"))), Summary: e.in.String("summary"), Examples: e.in.List("example")}
			skills := skillsOf(a)
			replaced := false
			for i := range skills {
				if skills[i].Name == s.Name {
					skills[i], replaced = s, true
				}
			}
			if !replaced {
				skills = append(skills, s)
			}
			return save(e, c, "skill_add", skills)
		}),
	}
	remove := &Command{
		Name: "remove", Summary: "Remove a skill", Mutating: true,
		Examples: []string{"myco agent skills remove --skill ios-release"},
		Flags: append([]Flag{
			{Name: "skill", Kind: String, Required: true, Help: "Name of the skill to remove"},
		}, target...),
		Run: handler(func(e *env) error {
			if err := e.in.Require("skill"); err != nil {
				return err
			}
			c, err := clientFor(e)
			if err != nil {
				return err
			}
			a, err := current(e, c)
			if err != nil {
				return err
			}
			name := strings.ToLower(strings.TrimSpace(e.in.String("skill")))
			var skills []api.Skill
			found := false
			for _, s := range skillsOf(a) {
				if s.Name == name {
					found = true
					continue
				}
				skills = append(skills, s)
			}
			if !found {
				return errorf("not_found", ExitNotFound, "list them with: myco agent skills list", "no skill %q on %s", name, a.Name)
			}
			return save(e, c, "skill_remove", skills)
		}),
	}
	set := &Command{
		Name: "set", Summary: "Replace all skills with a JSON list", Mutating: true,
		Examples: []string{
			`myco agent skills set --file skills.json`,
			`echo '[{"name":"code-review","summary":"Review Go and TypeScript changes"}]' | myco agent skills set --file -`,
			`myco agent skills set --file - <<< '[]'   # clear`,
		},
		Flags: append([]Flag{
			{Name: "file", Kind: String, Required: true, Help: `JSON list of {"name","summary","examples"} ('-' = stdin)`},
		}, target...),
		Run: handler(func(e *env) error {
			raw, err := e.readBody("json", "file")
			if err != nil {
				return err
			}
			var skills []api.Skill
			if err := json.Unmarshal([]byte(raw), &skills); err != nil {
				return errorf("invalid_argument", ExitUsage, `pass a JSON list such as [{"name":"code-review","summary":"..."}]`, "cannot read the skills: %v", err)
			}
			c, err := clientFor(e)
			if err != nil {
				return err
			}
			return save(e, c, "skills_set", skills)
		}),
	}
	return (&Command{Name: "skills", Summary: "What an agent can be asked to do: names, a line each, examples"}).Add(list, add, remove, set)
}

// skillsOf returns a's skills; a hub before skills only has capabilities.
func skillsOf(a *api.Agent) []api.Skill {
	if len(a.Skills) > 0 {
		return append([]api.Skill(nil), a.Skills...)
	}
	out := make([]api.Skill, 0, len(a.Capabilities))
	for _, c := range a.Capabilities {
		out = append(out, api.Skill{Name: c})
	}
	return out
}

func writeSkills(w io.Writer, a *api.Agent) {
	skills := skillsOf(a)
	if len(skills) == 0 {
		fmt.Fprintf(w, "%s has no skills yet. Add one: myco agent skills add --skill <name> --summary \"...\"\n", a.Name)
		return
	}
	fmt.Fprintf(w, "%s skills:\n", a.Name)
	for _, s := range skills {
		if s.Summary != "" {
			fmt.Fprintf(w, "  %s — %s\n", s.Name, s.Summary)
		} else {
			fmt.Fprintf(w, "  %s\n", s.Name)
		}
		for _, x := range s.Examples {
			fmt.Fprintf(w, "      e.g. %s\n", x)
		}
	}
}

// agentVisibilityCommand: who can find and use a node (people only).
func agentVisibilityCommand() *Command {
	return &Command{
		Name: "visibility", Summary: "Make a node visible to the team or private to its owner", Mutating: true,
		Examples: []string{
			"myco agent visibility --name laohu-claude --visibility private",
			"myco agent visibility --name laohu-claude --visibility team",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "The node"},
			{Name: "visibility", Kind: String, Required: true, Help: "team (everyone on the hub) or private (only its owner and the owner's agents)"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name", "visibility"); err != nil {
				return err
			}
			v := e.in.String("visibility")
			if v != "team" && v != "private" {
				return &UsageError{Message: fmt.Sprintf("invalid --visibility %q", v), Suggestion: "use team or private", Command: e.in.Cmd.Path()}
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "visibility", "agent": e.in.String("name"), "visibility": v})
			}
			a, err := c.SetVisibility(e.in.Ctx, e.in.String("name"), v)
			if err != nil {
				return err
			}
			return e.p.Print(a, func(w io.Writer) {
				if a.Visibility == "private" {
					fmt.Fprintf(w, "%s is now private: only its owner and the owner's agents can find it and give it tasks\n", a.Name)
				} else {
					fmt.Fprintf(w, "%s is now visible to everyone on the hub\n", a.Name)
				}
			})
		}),
	}
}

// agentCoordinatorCommand: let an agent edit its owner's other agents'
// cards (people only).
func agentCoordinatorCommand() *Command {
	return &Command{
		Name: "coordinator", Summary: "Let an agent edit the cards (description, labels, skills) of its owner's other agents", Mutating: true,
		Examples: []string{
			"myco agent coordinator --name cloud-hdkwmcki --on",
			"myco agent coordinator --name cloud-hdkwmcki --off",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "The agent"},
			{Name: "on", Kind: Bool, Help: "Make it a coordinator"},
			{Name: "off", Kind: Bool, Help: "Stop it being one"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			on, off := e.in.Bool("on"), e.in.Bool("off")
			if on == off {
				return &UsageError{Message: "pass --on or --off", Suggestion: "e.g. myco agent coordinator --name <agent> --on", Command: e.in.Cmd.Path()}
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "coordinator", "agent": e.in.String("name"), "coordinator": on})
			}
			a, err := c.SetCoordinator(e.in.Ctx, e.in.String("name"), on)
			if err != nil {
				return err
			}
			return e.p.Print(a, func(w io.Writer) {
				if a.Coordinator {
					fmt.Fprintf(w, "%s may now edit the description, labels and skills of %s's other agents\n", a.Name, a.Owner)
				} else {
					fmt.Fprintf(w, "%s is no longer a coordinator\n", a.Name)
				}
			})
		}),
	}
}
