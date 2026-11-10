package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

// projectClient: an agent's own token when this is an agent (its projects,
// what it may do as a member or coordinator), else admin or member
// credentials.
func (e *env) projectClient() (*client.Client, error) {
	if c, err := e.agentClient(); err == nil {
		return c, nil
	}
	return e.adminClient()
}

func projectCommands() *Command {
	nameFlag := Flag{Name: "name", Kind: String, Required: true, Help: "The project's short name, e.g. myco"}
	adminFile := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token"}

	list := &Command{
		Name: "list", Summary: "Projects you are in (--all: every project you can see)",
		Examples: []string{"myco project list", "myco project list --all --format json"},
		Flags:    []Flag{{Name: "all", Kind: Bool, Help: "Every project you can see, not only yours"}, adminFile},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			list, err := c.ListProjects(e.in.Ctx, !e.in.Bool("all"))
			if err != nil {
				return err
			}
			return e.p.Print(api.ProjectList{Projects: list}, func(w io.Writer) {
				if len(list) == 0 {
					fmt.Fprintln(w, "No projects. See all you can join: myco project list --all")
					return
				}
				for _, p := range list {
					state := ""
					if p.Archived {
						state = " (archived)"
					}
					fmt.Fprintf(w, "%-20s %s%s — %d nodes, %d documents\n", p.Name, p.Title, state, len(p.Members), len(p.Docs))
					if p.Goal != "" {
						fmt.Fprintf(w, "%-20s goal: %s\n", "", p.Goal)
					}
				}
			})
		}),
	}

	show := &Command{
		Name: "show", Summary: "A project's goal, brief, members (roles, skills, state), documents and links",
		Examples: []string{"myco project show --name myco", "myco project show --name myco --format json"},
		Flags:    []Flag{nameFlag, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			p, err := c.GetProject(e.in.Ctx, e.in.String("name"), false)
			if err != nil {
				return err
			}
			return e.p.Print(p, func(w io.Writer) { fmt.Fprint(w, projectMarkdown(p, false)) })
		}),
	}

	pull := &Command{
		Name: "pull", Summary: "Write a project's shared context to a local folder (PROJECT.md and its documents)",
		Long: `Writes <dir>/<project>/PROJECT.md (goal, brief, members with their roles and skills, links, the list of
documents) and <dir>/<project>/docs/<name>.md for each shared document. Run it again to refresh; files
the project no longer has are left alone.`,
		Examples: []string{"myco project pull --name myco", "myco project pull --name myco --dir docs/projects"},
		Flags:    []Flag{nameFlag, {Name: "dir", Kind: String, Help: "Folder to write into (default .myco/projects)"}, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			p, err := c.GetProject(e.in.Ctx, e.in.String("name"), true)
			if err != nil {
				return err
			}
			dir := filepath.Join(orDefault(e.in.String("dir"), filepath.Join(".myco", "projects")), p.Name)
			if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
				return err
			}
			files := []string{filepath.Join(dir, "PROJECT.md")}
			if err := os.WriteFile(files[0], []byte(projectMarkdown(p, true)), 0o644); err != nil {
				return err
			}
			for _, d := range p.Docs {
				f := filepath.Join(dir, "docs", d.Name+".md")
				if err := os.WriteFile(f, []byte(d.Body), 0o644); err != nil {
					return err
				}
				files = append(files, f)
			}
			return e.p.Print(map[string]any{"project": p.Name, "dir": dir, "files": files}, func(w io.Writer) {
				fmt.Fprintf(w, "wrote %d files to %s\n", len(files), dir)
				for _, f := range files {
					fmt.Fprintf(w, "  %s\n", f)
				}
			})
		}),
	}

	create := &Command{
		Name: "create", Summary: "Start a project (people; it is yours)", Mutating: true,
		Examples: []string{`myco project create --name myco --title "myco 开发" --goal "ship projects in the app"`},
		Flags: []Flag{nameFlag, {Name: "title", Kind: String, Help: "Display title"}, {Name: "goal", Kind: String, Help: "What the project should achieve, in a sentence or two"},
			{Name: "visibility", Kind: String, Help: "members (default: the people and nodes in it) or team (everyone on the hub)"}, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			req := api.CreateProjectRequest{Name: e.in.String("name"), Title: e.in.String("title"), Goal: e.in.String("goal"), Visibility: e.in.String("visibility")}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "project_create", "project": req})
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			p, err := c.CreateProject(e.in.Ctx, req)
			if err != nil {
				return err
			}
			return e.p.Print(p, func(w io.Writer) {
				fmt.Fprintf(w, "created project %s; add nodes with: myco project add --name %s --agent <node>\n", p.Name, p.Name)
			})
		}),
	}

	update := &Command{
		Name: "update", Summary: "Change a project's title, goal, brief or links (its owner and coordinators); visibility and archiving (people)", Mutating: true,
		Examples: []string{
			`myco project update --name myco --brief-file BRIEF.md`,
			`myco project update --name myco --link "repo=https://github.com/chenhg5/myco" --link "spec=https://example.com/spec"`,
			`myco project update --name myco --visibility team`,
		},
		Flags: []Flag{nameFlag, {Name: "title", Kind: String, Help: "Display title"}, {Name: "goal", Kind: String, Help: "Goal"},
			{Name: "brief-file", Kind: String, Help: "Read the brief (Markdown, up to 8 KB) from a file ('-' = stdin)"},
			{Name: "link", Kind: List, Help: "title=url; repeatable; replaces all links (--link '' clears them)"},
			{Name: "visibility", Kind: String, Help: "members or team (people)"},
			{Name: "archive", Kind: Bool, Help: "Archive it (people)"}, {Name: "unarchive", Kind: Bool, Help: "Bring it back (people)"}, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			var req api.UpdateProjectRequest
			if e.in.IsSet("title") {
				v := e.in.String("title")
				req.Title = &v
			}
			if e.in.IsSet("goal") {
				v := e.in.String("goal")
				req.Goal = &v
			}
			if e.in.IsSet("brief-file") {
				v, err := e.readBody("brief", "brief-file")
				if err != nil {
					return err
				}
				req.Brief = &v
			}
			if e.in.IsSet("link") {
				links := []api.ProjectLink{}
				for _, l := range e.in.List("link") {
					if l == "" {
						continue
					}
					title, url, ok := strings.Cut(l, "=")
					if !ok {
						title, url = "", l
					}
					links = append(links, api.ProjectLink{Title: title, URL: url})
				}
				req.Links = &links
			}
			if e.in.IsSet("visibility") {
				v := e.in.String("visibility")
				req.Visibility = &v
			}
			if e.in.Bool("archive") || e.in.Bool("unarchive") {
				v := e.in.Bool("archive")
				req.Archived = &v
			}
			if req == (api.UpdateProjectRequest{}) {
				return errorf("invalid_usage", ExitUsage, "pass --title, --goal, --brief-file, --link, --visibility or --archive", "nothing to update")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "project_update", "project": e.in.String("name"), "change": req})
			}
			c, err := e.projectClient()
			if req.Visibility != nil || req.Archived != nil {
				c, err = e.adminClient()
			}
			if err != nil {
				return err
			}
			p, err := c.UpdateProject(e.in.Ctx, e.in.String("name"), req)
			if err != nil {
				return err
			}
			return e.p.Print(p, func(w io.Writer) { fmt.Fprintf(w, "updated project %s\n", p.Name) })
		}),
	}

	memberCmd := func(verb string) *Command {
		cmd := &Command{
			Name: verb, Mutating: true,
			Flags: []Flag{nameFlag, {Name: "agent", Kind: String, Help: "The node (default: yourself)"}, adminFile},
		}
		switch verb {
		case "add":
			cmd.Summary = "Add a node to a project, or change its role or note (a node joins through its owner; a coordinator adds its own owner's nodes)"
			cmd.Examples = []string{
				`myco project add --name myco --agent wsl-codex --note "后端"`,
				`myco project add --name myco --agent cloud-lead --role coordinator   # people`,
				`myco project add --name myco   # join it yourself (an agent)`,
			}
			cmd.Flags = append(cmd.Flags, Flag{Name: "role", Kind: String, Help: "member or coordinator (people decide roles)"},
				Flag{Name: "note", Kind: String, Help: "What it does in this project"})
		case "remove":
			cmd.Summary = "Take a node out of a project"
			cmd.Examples = []string{"myco project remove --name myco --agent wsl-codex"}
		}
		cmd.Run = handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			agent := orDefault(e.in.String("agent"), e.cfg.Agent)
			if agent == "" {
				return errorf("invalid_usage", ExitUsage, "pass --agent <node>", "which node?")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "project_" + verb, "project": e.in.String("name"), "agent": agent})
			}
			// A person adding a node or setting a role uses their credentials;
			// an agent (joining, or coordinating) its own token.
			c, err := e.projectClient()
			if e.in.IsSet("role") {
				c, err = e.adminClient()
			}
			if err != nil {
				return err
			}
			var p *api.Project
			if verb == "add" {
				var req api.ProjectMemberRequest
				if e.in.IsSet("role") {
					v := e.in.String("role")
					req.Role = &v
				}
				if e.in.IsSet("note") {
					v := e.in.String("note")
					req.Note = &v
				}
				p, err = c.PutProjectMember(e.in.Ctx, e.in.String("name"), agent, req)
			} else {
				p, err = c.RemoveProjectMember(e.in.Ctx, e.in.String("name"), agent)
			}
			if err != nil {
				return err
			}
			return e.p.Print(p, func(w io.Writer) {
				if verb == "add" {
					fmt.Fprintf(w, "%s is in project %s\n", agent, p.Name)
				} else {
					fmt.Fprintf(w, "%s left project %s\n", agent, p.Name)
				}
			})
		})
		return cmd
	}

	docPut := &Command{
		Name: "put", Summary: "Write a shared document (its owner and coordinators)", Mutating: true,
		Examples: []string{"myco project doc put --name myco --doc conventions --file CONVENTIONS.md"},
		Flags:    []Flag{nameFlag, {Name: "doc", Kind: String, Required: true, Help: "Document name, e.g. design"}, {Name: "file", Kind: String, Required: true, Help: "Markdown file ('-' = stdin), up to 64 KB"}, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name", "doc", "file"); err != nil {
				return err
			}
			body, err := e.readBody("body", "file")
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "project_doc_put", "project": e.in.String("name"), "doc": e.in.String("doc"), "bytes": len(body)})
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			d, err := c.PutProjectDoc(e.in.Ctx, e.in.String("name"), e.in.String("doc"), body)
			if err != nil {
				return err
			}
			return e.p.Print(d, func(w io.Writer) { fmt.Fprintf(w, "wrote %s (%d bytes)\n", d.Name, d.Size) })
		}),
	}
	docShow := &Command{
		Name: "show", Summary: "Print a shared document",
		Examples: []string{"myco project doc show --name myco --doc conventions"},
		Flags:    []Flag{nameFlag, {Name: "doc", Kind: String, Required: true, Help: "Document name"}, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name", "doc"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			d, err := c.GetProjectDoc(e.in.Ctx, e.in.String("name"), e.in.String("doc"))
			if err != nil {
				return err
			}
			return e.p.Print(d, func(w io.Writer) { fmt.Fprintln(w, d.Body) })
		}),
	}
	docRemove := &Command{
		Name: "remove", Summary: "Remove a shared document (its owner and coordinators)", Mutating: true,
		Examples: []string{"myco project doc remove --name myco --doc old-notes"},
		Flags:    []Flag{nameFlag, {Name: "doc", Kind: String, Required: true, Help: "Document name"}, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name", "doc"); err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "project_doc_remove", "project": e.in.String("name"), "doc": e.in.String("doc")})
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			if err := c.DeleteProjectDoc(e.in.Ctx, e.in.String("name"), e.in.String("doc")); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"deleted": true}, func(w io.Writer) { fmt.Fprintln(w, "removed") })
		}),
	}
	del := &Command{
		Name: "delete", Summary: "Delete a project with its members and documents (people who manage it)", Mutating: true,
		Examples: []string{"myco project delete --name old-project"},
		Flags:    []Flag{nameFlag, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "project_delete", "project": e.in.String("name")})
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			if err := c.DeleteProject(e.in.Ctx, e.in.String("name")); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"deleted": true}, func(w io.Writer) { fmt.Fprintln(w, "deleted") })
		}),
	}
	doc := (&Command{Name: "doc", Summary: "A project's shared documents"}).Add(docShow, docPut, docRemove)
	return (&Command{Name: "project", Summary: "Projects: nodes working together on a goal, with a shared brief and documents"}).
		Add(list, show, pull, create, update, memberCmd("add"), memberCmd("remove"), doc, del)
}

// projectMarkdown is a project as Markdown (PROJECT.md, myco project show).
func projectMarkdown(p *api.Project, forFile bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", orDefault(p.Title, p.Name))
	if forFile {
		fmt.Fprintf(&b, "<!-- myco project %s; refresh with: myco project pull --name %s -->\n\n", p.Name, p.Name)
	}
	if p.Goal != "" {
		fmt.Fprintf(&b, "**Goal:** %s\n\n", p.Goal)
	}
	vis := "the people and nodes in it"
	if p.Visibility == "team" {
		vis = "everyone on the hub"
	}
	fmt.Fprintf(&b, "Project `%s`, owned by %s, visible to %s.", p.Name, orDefault(p.Owner, "the hub"), vis)
	if p.Archived {
		b.WriteString(" Archived.")
	}
	b.WriteString("\n\n")
	if p.Brief != "" {
		fmt.Fprintf(&b, "## Brief\n\n%s\n\n", p.Brief)
	}
	b.WriteString("## Members\n\n")
	if len(p.Members) == 0 {
		b.WriteString("No nodes yet.\n\n")
	}
	for _, m := range p.Members {
		role := ""
		if m.Role == "coordinator" {
			role = " (coordinator)"
		}
		line := fmt.Sprintf("- `%s`%s", m.Name, role)
		if m.Agent != nil {
			state := "offline"
			if m.Agent.Online {
				state = "online"
			}
			if m.Agent.DisplayName != "" {
				line += " " + m.Agent.DisplayName
			}
			line += ", " + state
		}
		if m.Note != "" {
			line += ": " + m.Note
		}
		b.WriteString(line + "\n")
		if m.Agent != nil {
			for _, s := range skillsOf(m.Agent) {
				if s.Summary != "" {
					fmt.Fprintf(&b, "  - skill `%s`: %s\n", s.Name, s.Summary)
				} else {
					fmt.Fprintf(&b, "  - skill `%s`\n", s.Name)
				}
			}
		}
	}
	b.WriteString("\n")
	if len(p.Links) > 0 {
		b.WriteString("## Links\n\n")
		for _, l := range p.Links {
			fmt.Fprintf(&b, "- [%s](%s)\n", orDefault(l.Title, l.URL), l.URL)
		}
		b.WriteString("\n")
	}
	if len(p.Docs) > 0 {
		b.WriteString("## Documents\n\n")
		for _, d := range p.Docs {
			if forFile {
				fmt.Fprintf(&b, "- [%s](docs/%s.md)\n", d.Name, d.Name)
			} else {
				fmt.Fprintf(&b, "- %s (%d bytes; myco project doc show --name %s --doc %s)\n", d.Name, d.Size, p.Name, d.Name)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
