package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/execnode"
)

// The join flow, for people who should not have to copy tokens around:
//
//	admin:    myco link create                 → https://hub/j/<token>
//	machine:  curl -fsSL https://hub/j/<token> | bash
//	          (installs myco, runs `myco machine join`, installs the runner service)
//	session:  myco join                        (or tell the agent: 接入 myco：<link>)

func linkCommands() *Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"}
	create := &Command{
		Name:     "create",
		Summary:  "Create a join link: one URL that adds machines (and their agent sessions) to the network",
		Mutating: true,
		Long: `Prints a one-line command to run on each machine to add, and a sentence to give AI sessions.
The link is a credential: anyone holding it can add machines until it expires, runs out of uses
or is revoked. Each machine gets its own credentials, so revoking the link does not disconnect
machines that already joined. With --exec, machines that join become exec nodes (tasks from
--allow-from run there as shell commands).`,
		Examples: []string{
			"myco link create",
			`myco link create --uses 1 --expires 24h --note "Glenn's laptop"`,
			"myco link create --expires 0 --uses 0 --note claude.ai   # for a cloud environment's setup script",
			"myco link create --exec --allow-from cloud-1 --uses 1    # one exec node",
		},
		Flags: []Flag{
			{Name: "expires", Kind: Duration, Default: "168h", Help: "Lifetime (0 = never)"},
			{Name: "uses", Kind: Int, Default: "10", Help: "Machines that may join with it (0 = unlimited; re-joining is free)"},
			{Name: "exec", Kind: Bool, Help: "Machines that join become exec nodes (requires --allow-from)"},
			{Name: "allow-from", Kind: List, Help: "Agents allowed to send tasks to agents joined through this link"},
			{Name: "label", Kind: List, Help: "Labels for every agent joined through this link (machine=<name> is added automatically)"},
			{Name: "capability", Kind: List, Help: "Default capabilities"},
			{Name: "note", Kind: String, Help: "Free-text note (who or what it is for)"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			req := api.CreateEnrollmentRequest{Kind: "link", AllowFrom: e.in.List("allow-from"), Labels: e.in.List("label"),
				Capabilities: e.in.List("capability"), Note: e.in.String("note"), MaxUses: e.in.Int("uses"),
				TTLSeconds: int64(e.in.Duration("expires").Seconds())}
			if e.in.Bool("exec") {
				req.Harness = execnode.Name
			}
			dry := e.in.Bool("dry-run")
			res, err := c.CreateEnrollment(e.in.Ctx, req, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "create_link", "link": res.Enrollment})
			}
			cmd := "curl -fsSL " + res.JoinURL + " | bash"
			out := map[string]any{"link": res.Enrollment, "join_url": res.JoinURL, "command": cmd,
				"agent_prompt": "接入 myco：" + res.JoinURL}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "join link %s: %s, %s\n\n", res.Enrollment.ID, linkValidity(res.Enrollment), linkUses(res.Enrollment))
				fmt.Fprintf(w, "On each machine to add (macOS / Linux / WSL / VPS), run:\n  %s\n\n", cmd)
				if e.in.Bool("exec") {
					fmt.Fprintf(w, "Machines that run it become exec nodes: tasks from %s run there as shell commands.\n\n",
						strings.Join(res.Enrollment.AllowFrom, ", "))
				} else {
					fmt.Fprintf(w, "To connect an AI session (Claude Code, Codex, claude.ai), tell it:\n  接入 myco：%s\n\n", res.JoinURL)
				}
				fmt.Fprintf(w, "Open the link in a browser for copy buttons. Keep it private; revoke with:\n  myco link revoke --id %s --force\n", res.Enrollment.ID)
			})
		}),
	}
	list := &Command{
		Name:     "list",
		Summary:  "List join links and the machines that joined through them (admin)",
		Examples: []string{"myco link list", "myco link list --format json"},
		Flags:    []Flag{adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			all, err := c.ListEnrollments(e.in.Ctx)
			if err != nil {
				return err
			}
			var links, machines []api.Enrollment
			for _, en := range all {
				switch en.Kind {
				case "link":
					links = append(links, en)
				case "machine":
					machines = append(machines, en)
				}
			}
			return e.p.Print(map[string]any{"links": nonNilSlice(links), "machines": nonNilSlice(machines)}, func(w io.Writer) {
				tw := newTable(w)
				fmt.Fprintln(tw, "LINK\tACTIVE\tUSES\tVALID\tEXEC\tMACHINES\tNOTE")
				for _, l := range links {
					var ms []string
					for _, m := range machines {
						if m.ParentID == l.ID {
							ms = append(ms, m.Machine)
						}
					}
					fmt.Fprintf(tw, "%s\t%v\t%s\t%s\t%v\t%s\t%s\n", l.ID, l.Active, linkUses(l), linkValidity(l), l.Harness == execnode.Name,
						dash(strings.Join(ms, ",")), truncate(l.Note, 40))
				}
				tw.Flush()
			})
		}),
	}
	revoke := &Command{
		Name:     "revoke",
		Summary:  "Revoke a join link; machines that already joined keep working",
		Mutating: true,
		Examples: []string{"myco link revoke --id enr_abc --force"},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "Link ID (from myco link list)"},
			{Name: "force", Kind: Bool, Help: "Required to actually revoke"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			if !dry && !e.in.Bool("force") {
				return errorf("confirmation_required", ExitUsage, "re-run with --force (or --dry-run)", "revoking link %s stops new machines from joining with it", e.in.String("id"))
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			res, err := c.RevokeEnrollment(e.in.Ctx, e.in.String("id"), dry)
			if err != nil {
				return err
			}
			if dry {
				res["action"] = "revoke_link"
				return e.dryRun(res)
			}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintf(w, "link %s revoked; machines that joined with it keep working (revoke one with: myco enrollment revoke --id <machine enrollment>)\n", e.in.String("id"))
			})
		}),
	}
	return (&Command{Name: "link", Summary: "Join links: add machines and agents without copying tokens (admin)"}).Add(create, list, revoke)
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func linkValidity(l api.Enrollment) string {
	if l.ExpiresAt == nil {
		return "never expires"
	}
	return "valid until " + l.ExpiresAt.Local().Format("2006-01-02 15:04")
}

func linkUses(l api.Enrollment) string {
	if l.MaxUses == 0 {
		return fmt.Sprintf("%d machines joined (no limit)", l.Uses)
	}
	return fmt.Sprintf("%d/%d machines", l.Uses, l.MaxUses)
}

func machineCommands() *Command {
	join := &Command{
		Name:     "join",
		Summary:  "Add this machine to the network with a join link (what the link's script runs)",
		Mutating: true,
		Long: `Exchanges the link for this machine's own credentials (~/.myco/machine.yaml, mode 600), labels its
agents machine=<name>, and installs the background runner service. Safe to re-run: the same machine
gets its credentials back without using up the link. Afterwards run "myco join" inside each agent
session that should be reachable. Links created with --exec instead turn this machine into an exec
node right away.`,
		Examples: []string{
			"curl -fsSL https://hub.example.com/j/<token> | bash      # the usual way",
			"myco machine join --link https://hub.example.com/j/<token>",
			"myco machine join --link https://hub.example.com/j/<token> --machine glenn-mac --no-service",
		},
		Flags: []Flag{
			{Name: "link", Kind: String, Help: "Join link URL (default: MYCO_HUB + MYCO_JOIN_TOKEN, as set by the link's script)"},
			{Name: "machine", Kind: String, Help: "Machine name (default: this host's name; \"cloud\" in claude.ai)"},
			{Name: "workdir", Kind: String, Help: "Exec links only: where commands run (default: your home directory)"},
			{Name: "no-service", Kind: Bool, Help: "Do not install the background runner service"},
		},
		Run: handler(func(e *env) error {
			hub, tok := strings.TrimRight(e.in.Getenv(config.EnvHub), "/"), e.in.Getenv(config.EnvJoinToken)
			if l := strings.TrimSpace(e.in.String("link")); l != "" {
				i := strings.LastIndex(l, "/j/")
				if i < 0 || !strings.HasPrefix(l, "http") {
					return errorf("invalid_argument", ExitUsage, "pass the whole link, e.g. https://hub.example.com/j/myco_enroll_...", "not a join link: %q", l)
				}
				hub, tok = l[:i], strings.Trim(l[i+3:], "/ ")
			}
			if hub == "" || tok == "" {
				return errorf("not_configured", ExitUsage, "pass --link <join link>, or run the link's script: curl -fsSL <link> | bash",
					"no join link given")
			}
			cloud := e.in.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID") != ""
			name := e.in.String("machine")
			if name == "" {
				name = hostname()
				if cloud {
					name = "cloud"
				}
			}
			c, err := client.New(hub, tok)
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			res, err := c.Join(e.in.Ctx, name, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "machine_join", "hub": hub, "machine": res})
			}
			m := &config.Machine{Hub: hub, Machine: res.Machine, EnrollToken: res.Token, NamePrefix: res.NamePrefix,
				Harness: res.Harness, AllowFrom: res.AllowFrom}
			if err := config.SaveMachine(e.defaultPath, m); err != nil {
				return err
			}
			out := map[string]any{"machine": m.Machine, "hub": hub, "created": res.Created, "labels": res.Labels,
				"config": config.MachinePath(e.defaultPath)}

			var notes []string
			if res.Harness == execnode.Name {
				agent, err := e.joinExecNode(m, e.in.String("workdir"))
				if err != nil {
					return err
				}
				out["exec_node"] = agent
				notes = append(notes, fmt.Sprintf("exec node %s ready: tasks from %s run here as %s in %s",
					agent.Name, strings.Join(agent.AllowFrom, ", "), orDefault(e.in.Getenv("USER"), "this user"), agent.Binding.Workdir))
			}
			switch {
			case cloud:
				notes = append(notes, "claude.ai cloud: no background service here. Run `myco join` in this session, then keep `myco inbox wait --timeout 55m` running as a background command.")
			case e.in.Bool("no-service"):
				notes = append(notes, "service not installed (--no-service): run `myco runner start --all` to serve this machine's agents")
			default:
				if sr, err := e.installService(3, "info", false); err != nil {
					notes = append(notes, "could not install the background service ("+toError(err, "").Message+"); keep `myco runner start --all` running instead, e.g. in tmux")
				} else {
					out["service"] = sr.Plan.Manager
					notes = append(notes, "background runner installed ("+sr.Plan.Manager+"); it wakes this machine's agents when messages arrive")
				}
			}
			out["clis"] = detectCLIs()
			return e.p.Print(out, func(w io.Writer) {
				verb := "joined"
				if !res.Created {
					verb = "re-joined (new credentials issued)"
				}
				fmt.Fprintf(w, "==> machine %q %s %s (labels: %s)\n", m.Machine, verb, hub, strings.Join(res.Labels, ", "))
				for _, n := range notes {
					fmt.Fprintln(w, "==> "+n)
				}
				if res.Harness != execnode.Name && !cloud {
					fmt.Fprintln(w)
					fmt.Fprintln(w, "Next: in any Claude Code or Codex session on this machine, say \"接入 myco\", or run:")
					fmt.Fprintln(w, "  myco join")
					if found := detectCLIs(); len(found) > 0 {
						fmt.Fprintf(w, "(found here: %s)\n", strings.Join(found, ", "))
					}
				}
			})
		}),
	}
	return (&Command{Name: "machine", Summary: "This machine's membership in the network"}).Add(join)
}

// detectCLIs lists the agent CLIs installed here.
func detectCLIs() []string {
	var out []string
	for _, n := range []string{"claude", "codex"} {
		if _, err := exec.LookPath(n); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// joinExecNode enrolls this user on this machine as the machine's exec node.
func (e *env) joinExecNode(m *config.Machine, workdir string) (*api.Agent, error) {
	if workdir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		workdir = home
	}
	name := m.NamePrefix + "shell"
	b, err := execBinding(e, name, workdir)
	if err != nil {
		return nil, err
	}
	res, err := e.enroll(m.Hub, m.EnrollToken, api.EnrollRequest{Name: name, Binding: b,
		Description: "exec node on " + m.Machine + " (" + workdir + ")"}, false)
	if err != nil {
		return nil, err
	}
	return &res.Agent, nil
}

// execBinding is the identity of an exec node: this user on this machine
// under this name (so re-enrolling recovers it).
func execBinding(e *env, name, workdir string) (api.Binding, error) {
	wd, err := filepath.Abs(orDefault(workdir, "."))
	if err != nil {
		return api.Binding{}, err
	}
	if st, err := os.Stat(wd); err != nil || !st.IsDir() {
		return api.Binding{}, errorf("not_found", ExitNotFound, "create it first, or pass another --workdir", "workdir %s does not exist", wd)
	}
	host, user := hostname(), orDefault(e.in.Getenv("USER"), "user")
	return api.Binding{Harness: execnode.Name, Environment: orDefault(e.in.String("environment"), host), Workdir: wd,
		SessionID: "exec:" + host + ":" + user + ":" + name}, nil
}

func joinCommand() *Command {
	return &Command{
		Name:     "join",
		Summary:  "Register the current agent session (Claude Code, Codex, claude.ai) on this machine's network",
		Mutating: true,
		Long: `Run it inside the session that should become an agent. It detects the session, names the agent
<machine>-<cli>-<id> and stores its credentials; the machine's runner service then wakes the session
when messages arrive. Needs the machine to have joined first (its join link's script does that).
Safe to re-run: the same session gets the same agent back.`,
		Examples: []string{
			"myco join",
			`myco join --description "reviews PRs for the web app" --display-name "Web reviewer"`,
			"myco join --harness codex --session-id 0199a1b2-...   # if detection fails",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Help: "Agent name (default: <machine>-<cli>-<short session id>)"},
			{Name: "display-name", Kind: String, Help: "Human-friendly name"},
			{Name: "description", Kind: String, Help: "What this agent does (helps others pick it)"},
			{Name: "label", Kind: List, Help: "Extra labels, e.g. group=web"},
			{Name: "harness", Kind: String, Help: "claude or codex (default: detected)"},
			{Name: "session-id", Kind: String, Help: "Session id (default: detected)"},
		},
		Run: handler(func(e *env) error {
			m, err := config.LoadMachine(e.defaultPath)
			if err != nil {
				return err
			}
			hub, tok := "", e.in.Getenv(config.EnvEnrollToken)
			prefix := ""
			if m != nil {
				hub, prefix = m.Hub, m.NamePrefix
				if tok == "" {
					tok = m.EnrollToken
				}
			}
			if h := e.in.Getenv(config.EnvHub); h != "" && (m == nil || tok != m.EnrollToken) {
				hub = h
			}
			if hub == "" || tok == "" {
				return errorf("not_configured", ExitUsage, "add this machine first: run the join link's command (curl -fsSL <link> | bash), or ask your admin for one (myco link create)",
					"this machine has not joined a myco network")
			}
			if m != nil && m.Harness == execnode.Name {
				return errorf("invalid_argument", ExitUsage, "sessions join through a normal link; this machine joined as an exec node",
					"machine %s is an exec node", m.Machine)
			}
			b, how, err := detectSession(e)
			if err != nil {
				return err
			}
			name := e.in.String("name")
			if name == "" && prefix != "" {
				name = prefix + shortHarness(b.Harness) + "-" + slug(b.SessionID, 4)
			}
			desc := e.in.String("description")
			if desc == "" {
				desc = fmt.Sprintf("%s session on %s in %s", shortHarness(b.Harness), orDefault(machineName(m), b.Environment), b.Workdir)
			}
			dry := e.in.Bool("dry-run")
			res, err := e.enroll(hub, tok, api.EnrollRequest{Name: name, DisplayName: e.in.String("display-name"), Description: desc,
				Labels: e.in.List("label"), Binding: b}, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "join", "agent": res.Agent, "detected": how})
			}
			cloud := b.Harness == "claude-cloud"
			out := map[string]any{"agent": res.Agent, "created": res.Created, "detected": how, "config": e.cfgPath}
			return e.p.Print(out, func(w io.Writer) {
				verb := "joined as"
				if !res.Created {
					verb = "already joined as"
				}
				fmt.Fprintf(w, "%s %s (%s)\n", verb, res.Agent.Name, how)
				if cloud {
					fmt.Fprintln(w, "To stay reachable, keep this running as a background command and start it again whenever it exits:")
					fmt.Fprintln(w, "  myco inbox wait --timeout 55m")
				} else {
					fmt.Fprintln(w, "The machine's runner service will wake this session when a message arrives (within ~30s of joining).")
				}
				fmt.Fprintln(w, "Others can now reach it: myco message send --to "+res.Agent.Name+" --body \"...\"")
			})
		}),
	}
}

func machineName(m *config.Machine) string {
	if m == nil {
		return ""
	}
	return m.Machine
}

func shortHarness(h string) string {
	switch h {
	case "claude-cloud":
		return "claude"
	case "":
		return "agent"
	}
	return h
}

// slug keeps the last n letters/digits of id, lowercased.
func slug(id string, n int) string {
	var r []rune
	for _, c := range strings.ToLower(id) {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			r = append(r, c)
		}
	}
	if len(r) > n {
		r = r[len(r)-n:]
	}
	return string(r)
}

// detectSession finds the agent session this command runs in: explicit
// flags, then Claude Code (local or cloud) environment variables, then the
// Codex session most recently active in this directory.
func detectSession(e *env) (api.Binding, string, error) {
	wd, _ := os.Getwd()
	if id := e.in.String("session-id"); id != "" {
		h := e.in.String("harness")
		if h == "" {
			return api.Binding{}, "", errorf("invalid_argument", ExitUsage, "add --harness claude or --harness codex", "--session-id needs --harness")
		}
		return api.Binding{Harness: h, SessionID: id, Environment: hostname(), Workdir: wd}, "given by flags", nil
	}
	if want := e.in.String("harness"); want == "" || want == "claude" || want == "claude-cloud" {
		if b := detectBinding(e.in.Getenv); b.SessionID != "" {
			return b, "detected " + b.Harness + " session " + b.SessionID, nil
		}
	}
	if want := e.in.String("harness"); want == "" || want == "codex" {
		inCodex := false
		for _, kv := range os.Environ() {
			if strings.HasPrefix(kv, "CODEX_") {
				inCodex = true
				break
			}
		}
		if id, file, ok := findCodexSession(wd, inCodex, time.Now()); ok {
			return api.Binding{Harness: "codex", SessionID: id, Environment: hostname(), Workdir: wd},
				"detected codex session " + id + " (" + filepath.Base(file) + ")", nil
		}
	}
	return api.Binding{}, "", errorf("not_found", ExitNotFound,
		"run this inside a Claude Code or Codex session; if it still fails pass --harness <claude|codex> --session-id <id> (never guess the id)",
		"no agent session detected here")
}

// findCodexSession returns the Codex session that was active most recently
// (last 3 minutes) in dir — the one running this command. Outside an
// obviously-Codex process the session must also have started in dir.
func findCodexSession(dir string, inCodex bool, now time.Time) (id, file string, ok bool) {
	pattern := filepath.Join(harness.HomeDir("CODEX_HOME", ".codex"), "sessions", "*", "*", "*", "rollout-*.jsonl")
	files, _ := filepath.Glob(pattern)
	type cand struct {
		path, id string
		mod      time.Time
		sameDir  bool
	}
	var cs []cand
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil || now.Sub(st.ModTime()) > 3*time.Minute {
			continue
		}
		sid, cwd := readCodexMeta(f)
		if sid == "" {
			base := strings.TrimSuffix(filepath.Base(f), ".jsonl")
			if len(base) >= 36 {
				sid = base[len(base)-36:]
			}
		}
		if sid == "" {
			continue
		}
		cs = append(cs, cand{f, sid, st.ModTime(), cwd != "" && sameDir(cwd, dir)})
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].sameDir != cs[j].sameDir {
			return cs[i].sameDir
		}
		return cs[i].mod.After(cs[j].mod)
	})
	for _, c := range cs {
		if c.sameDir || inCodex {
			return c.id, c.path, true
		}
	}
	return "", "", false
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 == nil && err2 == nil {
		return ra == rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// readCodexMeta reads the session id and cwd from a rollout's first line
// ({"type":"session_meta","payload":{"id":...,"cwd":...}}).
func readCodexMeta(path string) (id, cwd string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	if !sc.Scan() {
		return "", ""
	}
	var line struct {
		Type    string `json:"type"`
		Payload struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal(sc.Bytes(), &line) != nil {
		return "", ""
	}
	return line.Payload.ID, line.Payload.Cwd
}
