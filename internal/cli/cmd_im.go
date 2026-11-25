package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mdp/qrterminal/v3"
	"golang.org/x/term"
	"rsc.io/qr"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/im"
	"github.com/myco-ai/myco/internal/im/feishu"
	imslack "github.com/myco-ai/myco/internal/im/slack"
)

func imCommands() *Command {
	g := &Command{Name: "im", Summary: "Chat-app bots (Feishu/Lark, Slack) that pass messages to your agents",
		Long: `A bridge is a bot in a chat app that you talk to like an agent: what you write
becomes a task for an agent (the default one, or /to <name>), its progress
updates one message in place, and its answer comes back in the same chat.
While a task is open, what you write is added to it, and answers its question.

The bot speaks as a node of yours on the hub (im-<name>, receiving through its
inbox). It connects out to the chat app (Feishu long connection, Slack Socket
Mode): no public address is needed. This machine's runner service serves the
bridges configured here; myco im serve runs them in the foreground.`}
	g.Add(imConnectCommand(), imServeCommand(), imListCommand(), imAllowCommand(), imSetCommand(), imRemoveCommand())
	return g
}

func (e *env) imDir() string { return im.Dir(config.Dir(e.defaultPath)) }

var imPlatforms = []string{"feishu", "lark", "slack"}

func imConnectCommand() *Command {
	return &Command{Name: "connect", Summary: "Create or connect a bot: Feishu/Lark by scanning a QR code, Slack from an app manifest", Mutating: true,
		Args: "<feishu|lark|slack>",
		Examples: []string{
			"myco im connect feishu --agent coder            # scan the QR code with the Feishu app",
			"myco im connect feishu --app-id cli_xxx --app-secret-file secret.txt --agent coder",
			"myco im connect slack --agent coder             # prints the link that creates the Slack app",
			"myco im connect slack --bot-token-file bot.txt --app-token-file app.txt --agent coder",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Help: "Bridge name (default: the platform), 1–32 lowercase letters, digits or hyphens"},
			{Name: "agent", Kind: String, Help: "Agent that gets what is written to the bot (change later with /agent in the chat or myco im set)"},
			{Name: "project", Kind: String, Help: "Put new conversations in this project"},
			{Name: "lang", Kind: Enum, Values: []string{"zh", "en"}, Help: "What the bot speaks (default: zh for Feishu, en otherwise)"},
			{Name: "app-id", Kind: String, Help: "Feishu/Lark: an existing app's App ID (skips the QR code)"},
			{Name: "app-secret-file", Kind: String, Help: "Feishu/Lark: file with that app's App Secret"},
			{Name: "bot-token-file", Kind: String, Env: "MYCO_SLACK_BOT_TOKEN_FILE", Help: "Slack: file with the Bot User OAuth Token (xoxb-…)"},
			{Name: "app-token-file", Kind: String, Env: "MYCO_SLACK_APP_TOKEN_FILE", Help: "Slack: file with the app-level token (xapp-…, scope connections:write)"},
			{Name: "qr-png", Kind: String, Help: "Feishu/Lark: also save the QR code as a PNG here"},
			{Name: "timeout", Kind: Duration, Default: "10m", Help: "Feishu/Lark: how long to wait for the scan"},
			{Name: "profile", Kind: String, Help: "Speak as this existing agent profile instead of a new im-<name> node"},
		},
		Run: handler(func(e *env) error {
			if len(e.in.Args) != 1 {
				return errorf("invalid_usage", ExitUsage, "myco im connect feishu (or lark, slack)", "name one platform")
			}
			platform := strings.ToLower(e.in.Args[0])
			if !slices.Contains(imPlatforms, platform) {
				return errorf("invalid_argument", ExitUsage, "use feishu, lark or slack", "unknown platform %q", platform)
			}
			name := e.in.String("name")
			if name == "" {
				name = platform
			}
			if !im.ValidName(name) {
				return errorf("invalid_argument", ExitUsage, "use 1–32 lowercase letters, digits or hyphens", "invalid bridge name %q", name)
			}
			dir := e.imDir()
			if old, err := im.Load(dir, name); err == nil && old != nil {
				return errorf("conflict", ExitConflict, "choose another --name, or remove it first: myco im remove --name "+name,
					"a bridge named %q is already configured", name)
			}
			dry := e.in.Bool("dry-run")
			cfg := &im.Config{Name: name, Platform: platform, Agent: e.in.String("agent"), Project: e.in.String("project"), Lang: e.in.String("lang")}
			ctx := e.in.Ctx
			// The node it will speak as, checked first: no scan for nothing.
			if _, _, err := e.bridgeNode(cfg, true); err != nil {
				return err
			}
			var check func(context.Context) (string, error)
			switch platform {
			case "feishu", "lark":
				if id := e.in.String("app-id"); id != "" {
					secret, err := readSecretFile(e.in.String("app-secret-file"), "--app-secret-file")
					if err != nil {
						return err
					}
					cfg.AppID, cfg.AppSecret = id, secret
				} else if dry {
					return e.dryRun(map[string]any{"action": "im connect", "platform": platform, "name": name,
						"steps": []string{"show a QR code to scan with the Feishu/Lark app", "create node im-" + name, "save " + filepath.Join(dir, name+".json")}})
				} else {
					reg, err := e.scanFeishu(ctx, e.in.String("qr-png"), e.in.Duration("timeout"))
					if err != nil {
						return err
					}
					cfg.AppID, cfg.AppSecret, cfg.Owner, cfg.Platform = reg.AppID, reg.AppSecret, reg.OwnerOpenID, reg.Platform
				}
				ad := feishu.New(cfg.Platform, cfg.AppID, cfg.AppSecret, nil)
				check = ad.Check
			case "slack":
				bot, err := e.slackToken("bot-token-file", "MYCO_SLACK_BOT_TOKEN", "Bot User OAuth Token (xoxb-…)")
				if err != nil {
					return err
				}
				app, err := e.slackToken("app-token-file", "MYCO_SLACK_APP_TOKEN", "app-level token (xapp-…)")
				if err != nil {
					return err
				}
				if bot == "" || app == "" {
					return errorf("not_configured", ExitUsage,
						"1) open "+imslack.CreateURL("myco")+" and create the app (the manifest is filled in); "+
							"2) install it to your workspace and copy the Bot User OAuth Token (OAuth & Permissions); "+
							"3) under Basic Information, create an app-level token with connections:write; "+
							"4) run again with --bot-token-file and --app-token-file (or paste them when asked)",
						"Slack tokens are needed")
				}
				cfg.BotToken, cfg.AppToken = bot, app
				check = imslack.New(bot, app, nil).Check
			}
			if cfg.Owner == "" {
				cfg.Claim = newClaimCode()
			}
			cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			botName, err := check(cctx)
			cancel()
			if err != nil {
				return errorf("invalid_argument", ExitUsage, "check the app's credentials (and that the app is published)", "the %s app does not accept these credentials: %v", platform, err)
			}
			profile, owner, err := e.bridgeNode(cfg, dry)
			if err != nil {
				return err
			}
			cfg.Profile, cfg.OwnerName = profile, owner
			if err := cfg.Validate(); err != nil {
				return errorf("invalid_argument", ExitUsage, "", "%v", err)
			}
			if dry {
				return e.dryRun(map[string]any{"action": "im connect", "bridge": cfg.Redacted(), "bot": botName})
			}
			if err := im.Save(dir, cfg); err != nil {
				return err
			}
			out := map[string]any{"bridge": cfg.Redacted(), "bot": botName, "config": filepath.Join(dir, name+".json"),
				"serve": "served by this machine's runner service within 30s; or run: myco im serve --name " + name}
			if cfg.Claim != "" {
				out["claim"] = "send /claim " + cfg.Claim + " to the bot in a direct message to become its owner"
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "connected %s bot %q as bridge %s\n", platform, botName, name)
				if cfg.Agent == "" {
					fmt.Fprintln(w, "no default agent yet: write /agent <name> to the bot, or run myco im set --name "+name+" --agent <name>")
				}
				if cfg.Claim != "" {
					fmt.Fprintf(w, "to become its owner, send this to the bot in a direct message: /claim %s\n", cfg.Claim)
				}
				fmt.Fprintln(w, "this machine's runner service starts it within 30 seconds; or run: myco im serve --name "+name)
			})
		}),
	}
}

// scanFeishu shows the registration QR code and waits for the scan.
func (e *env) scanFeishu(ctx context.Context, png string, timeout time.Duration) (*feishu.Registration, error) {
	p, err := (&feishu.Registrar{}).Begin(ctx)
	if err != nil {
		return nil, errorf("unavailable", ExitUnavailable, "check that this machine reaches accounts.feishu.cn, or pass --app-id with --app-secret-file", "Feishu registration: %v", err)
	}
	w := e.in.Stderr
	fmt.Fprintln(w, "Scan with the Feishu or Lark app to create the bot (用飞书或 Lark 手机 App 扫码创建机器人):")
	fmt.Fprintln(w, p.URL)
	qrterminal.GenerateWithConfig(p.URL, qrterminal.Config{Level: qrterminal.M, Writer: w, HalfBlocks: true,
		BlackChar: qrterminal.BLACK_BLACK, WhiteChar: qrterminal.WHITE_WHITE, BlackWhiteChar: qrterminal.BLACK_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK, QuietZone: 2})
	if png != "" {
		if code, err := qr.Encode(p.URL, qr.M); err == nil {
			code.Scale = 8
			if err := os.WriteFile(png, code.PNG(), 0o600); err != nil {
				fmt.Fprintf(w, "could not save the QR code: %v\n", err)
			} else {
				fmt.Fprintf(w, "QR code saved to %s\n", png)
			}
		}
	}
	fmt.Fprintln(w, "waiting for the scan…")
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	reg, err := p.Wait(wctx)
	if err != nil {
		return nil, errorf("timeout", ExitTimeout, "run the command again for a new QR code", "Feishu registration: %v", err)
	}
	return reg, nil
}

// slackToken reads a token from its file flag, an environment variable, or
// (on a terminal) a prompt.
func (e *env) slackToken(flag, envName, what string) (string, error) {
	if f := e.in.String(flag); f != "" {
		return readSecretFile(f, "--"+flag)
	}
	if v := strings.TrimSpace(e.in.Getenv(envName)); v != "" {
		return v, nil
	}
	if f, ok := e.in.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprintf(e.in.Stderr, "Paste the %s (leave empty to see how to create the app): ", what)
		line, _ := bufio.NewReader(f).ReadString('\n')
		return strings.TrimSpace(line), nil
	}
	return "", nil
}

func readSecretFile(path, flag string) (string, error) {
	if path == "" {
		return "", errorf("invalid_argument", ExitUsage, "pass "+flag+" <file>", "%s is required", flag)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errorf("not_found", ExitNotFound, "", "reading %s: %v", flag, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func newClaimCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// bridgeNode creates (or reuses) the node a bridge speaks as and returns its
// profile and its owner's member name.
func (e *env) bridgeNode(cfg *im.Config, dry bool) (string, string, error) {
	if p := e.in.String("profile"); p != "" {
		c, err := config.Load(p)
		if err != nil || c.Token == "" {
			return "", "", errorf("not_found", ExitNotFound, "pass the path of an agent profile (~/.myco/agents/<name>.yaml)", "no agent credentials in %s", p)
		}
		owner := ""
		if cl, err := client.New(c.Hub, c.Token); err == nil {
			if me, err := cl.Me(e.in.Ctx); err == nil {
				owner = me.Owner
			}
		}
		return p, owner, nil
	}
	hub, tok, prefix := "", e.in.Getenv(config.EnvEnrollToken), ""
	if mf, _ := config.LoadMachine(e.defaultPath); mf != nil {
		want := e.in.String("hub")
		if want == "" {
			want = e.in.Getenv(config.EnvHub)
		}
		if h, ok := mf.For(want); ok {
			hub, tok, prefix = h.Hub, h.EnrollToken, h.NamePrefix
		} else if mf.Hub != "" {
			hub, tok, prefix = mf.Hub, mf.EnrollToken, mf.NamePrefix
		}
	}
	if hub == "" {
		hub = e.in.Getenv(config.EnvHub)
		if hub == "" {
			hub = e.in.String("hub")
		}
	}
	if hub == "" || tok == "" {
		return "", "", errorf("not_configured", ExitUsage,
			"add this machine to your myco network first (run its join link), or pass --profile with an agent profile",
			"this machine has not joined a myco network")
	}
	c, err := client.New(hub, tok)
	if err != nil {
		return "", "", err
	}
	pname := map[string]string{"feishu": "Feishu", "lark": "Lark", "slack": "Slack"}[cfg.Platform]
	b := api.Binding{Harness: "inbox", SessionID: "im:" + cfg.Platform + ":" + cfg.Name, Environment: hostname()}
	res, err := c.Enroll(e.in.Ctx, api.EnrollRequest{Name: prefix + "im-" + cfg.Name, DisplayName: pname + " bot",
		Description: pname + " bridge on " + hostname() + ": passes chat messages to agents", Binding: b}, dry)
	if err != nil {
		return "", "", err
	}
	path := profilePathFor(e.defaultPath, hub, res.Agent.Name)
	if dry {
		return path, res.Agent.Owner, nil
	}
	pc, err := config.Load(path)
	if err != nil {
		pc = &config.Config{}
	}
	pc.Hub, pc.Agent, pc.Token, pc.Harness, pc.SessionID, pc.Replaced = hub, res.Agent.Name, res.Token, b.Harness, b.SessionID, ""
	if err := config.Save(path, pc); err != nil {
		return "", "", err
	}
	return path, res.Agent.Owner, nil
}

func imServeCommand() *Command {
	return &Command{Name: "serve", Summary: "Run bridges in the foreground (the runner service also runs them)",
		Examples: []string{"myco im serve", "myco im serve --name feishu"},
		Flags:    []Flag{{Name: "name", Kind: String, Help: "Only this bridge (default: all configured here)"}},
		Run: handler(func(e *env) error {
			names, err := im.List(e.imDir())
			if err != nil {
				return err
			}
			if n := e.in.String("name"); n != "" {
				if !slices.Contains(names, n) {
					return errorf("not_found", ExitNotFound, "see myco im list", "no bridge named %q", n)
				}
				names = []string{n}
			}
			if len(names) == 0 {
				return errorf("not_found", ExitNotFound, "create one: myco im connect feishu (or slack)", "no bridges configured in %s", e.imDir())
			}
			log := slog.New(slog.NewTextHandler(e.in.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
			var wg sync.WaitGroup
			errs := make(chan error, len(names))
			for _, n := range names {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := e.runBridge(e.in.Ctx, n, log); err != nil {
						log.Error("bridge stopped", "bridge", n, "err", err)
						errs <- err
					}
				}()
			}
			wg.Wait()
			close(errs)
			if err := <-errs; err != nil && len(names) == 1 {
				return err
			}
			return nil
		}),
	}
}

// runBridge runs one configured bridge until ctx ends.
func (e *env) runBridge(ctx context.Context, name string, log *slog.Logger) error {
	dir := e.imDir()
	cfg, err := im.Load(dir, name)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	pc, err := config.Load(cfg.Profile)
	if err != nil || pc.Token == "" {
		return fmt.Errorf("bridge %s: no credentials in %s", name, cfg.Profile)
	}
	c, err := client.New(pc.Hub, pc.Token)
	if err != nil {
		return err
	}
	c = e.withSealer(c, cfg.Profile, pc)
	log = log.With("bridge", name)
	var ad im.Adapter
	switch cfg.Platform {
	case "feishu", "lark":
		ad = feishu.New(cfg.Platform, cfg.AppID, cfg.AppSecret, log)
	case "slack":
		ad = imslack.New(cfg.BotToken, cfg.AppToken, log)
	}
	b, err := im.New(*cfg, ad, im.ClientHub{C: c}, dir, log)
	if err != nil {
		return err
	}
	b.OnConfig = func(nc im.Config) error { return im.Save(dir, &nc) }
	log.Info("bridge serving", "platform", cfg.Platform, "agent", cfg.Agent, "node", pc.Agent)
	return b.Run(ctx)
}

// serveBridges runs the bridges configured on this machine and starts new
// ones as they appear (the runner service calls it).
func (e *env) serveBridges(ctx context.Context, log *slog.Logger) {
	started := map[string]bool{}
	var mu sync.Mutex
	scan := func() {
		names, err := im.List(e.imDir())
		if err != nil {
			log.Warn("reading bridges failed", "err", err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, n := range names {
			if started[n] {
				continue
			}
			started[n] = true
			go func() {
				for ctx.Err() == nil {
					err := e.runBridge(ctx, n, log)
					if ctx.Err() != nil {
						return
					}
					if _, lerr := im.Load(e.imDir(), n); lerr != nil {
						mu.Lock()
						delete(started, n) // removed
						mu.Unlock()
						return
					}
					log.Error("bridge stopped; restarting in a minute", "bridge", n, "err", err)
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Minute):
					}
				}
			}()
		}
	}
	scan()
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			scan()
		}
	}
}

func imListCommand() *Command {
	return &Command{Name: "list", Summary: "Bridges configured on this machine (no secrets shown)",
		Run: handler(func(e *env) error {
			dir := e.imDir()
			names, err := im.List(dir)
			if err != nil {
				return err
			}
			var out []map[string]any
			for _, n := range names {
				if c, err := im.Load(dir, n); err == nil {
					out = append(out, c.Redacted())
				}
			}
			return e.p.Print(map[string]any{"bridges": nonNilSlice(out), "dir": dir}, func(w io.Writer) {
				if len(out) == 0 {
					fmt.Fprintln(w, "no bridges; create one: myco im connect feishu (or slack)")
				}
				for _, b := range out {
					fmt.Fprintf(w, "%-12s %-7s agent=%v owner=%v\n", b["name"], b["platform"], orDash(fmt.Sprint(b["agent"])), orDash(fmt.Sprint(b["owner"])))
				}
			})
		}),
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (e *env) loadBridge() (*im.Config, error) {
	name := e.in.String("name")
	c, err := im.Load(e.imDir(), name)
	if err != nil {
		return nil, errorf("not_found", ExitNotFound, "see myco im list", "no bridge named %q", name)
	}
	return c, nil
}

func imAllowCommand() *Command {
	return &Command{Name: "allow", Summary: "Let another chat-app user give tasks through a bridge (or stop letting them)", Mutating: true,
		Examples: []string{"myco im allow --name feishu --user ou_123", "myco im allow --name slack --user U0123 --remove"},
		Flags: []Flag{{Name: "name", Kind: String, Required: true, Help: "Bridge name"},
			{Name: "user", Kind: String, Required: true, Help: "Their user id in the chat app (they see it with /whoami); * for everyone"},
			{Name: "remove", Kind: Bool, Help: "Remove them instead"}},
		Run: handler(func(e *env) error {
			c, err := e.loadBridge()
			if err != nil {
				return err
			}
			u := strings.TrimSpace(e.in.String("user"))
			c.AllowFrom = slices.DeleteFunc(c.AllowFrom, func(s string) bool { return s == u })
			if !e.in.Bool("remove") {
				c.AllowFrom = append(c.AllowFrom, u)
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "im allow", "bridge": c.Redacted()})
			}
			if err := im.Save(e.imDir(), c); err != nil {
				return err
			}
			return e.p.Print(c.Redacted(), func(w io.Writer) {
				fmt.Fprintf(w, "%s allows: owner %s %s (a running bridge picks this up when restarted)\n", c.Name, orDash(c.Owner), strings.Join(c.AllowFrom, " "))
			})
		}),
	}
}

func imSetCommand() *Command {
	return &Command{Name: "set", Summary: "Change a bridge's default agent, project or language", Mutating: true,
		Examples: []string{"myco im set --name feishu --agent reviewer"},
		Flags: []Flag{{Name: "name", Kind: String, Required: true, Help: "Bridge name"},
			{Name: "agent", Kind: String, Help: "Default agent"},
			{Name: "project", Kind: String, Help: "Project for new conversations (\"\" for none)"},
			{Name: "lang", Kind: Enum, Values: []string{"zh", "en"}, Help: "What the bot speaks"}},
		Run: handler(func(e *env) error {
			c, err := e.loadBridge()
			if err != nil {
				return err
			}
			if e.in.IsSet("agent") {
				c.Agent = e.in.String("agent")
			}
			if e.in.IsSet("project") {
				c.Project = e.in.String("project")
			}
			if e.in.IsSet("lang") {
				c.Lang = e.in.String("lang")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "im set", "bridge": c.Redacted()})
			}
			if err := im.Save(e.imDir(), c); err != nil {
				return err
			}
			return e.p.Print(c.Redacted(), func(w io.Writer) {
				fmt.Fprintf(w, "%s: agent=%s project=%s (a running bridge picks this up when restarted)\n", c.Name, orDash(c.Agent), orDash(c.Project))
			})
		}),
	}
}

func imRemoveCommand() *Command {
	return &Command{Name: "remove", Summary: "Remove a bridge from this machine (the app and its hub node stay)", Mutating: true,
		Flags: []Flag{{Name: "name", Kind: String, Required: true, Help: "Bridge name"}},
		Run: handler(func(e *env) error {
			c, err := e.loadBridge()
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "im remove", "bridge": c.Redacted()})
			}
			if err := im.Remove(e.imDir(), c.Name); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"removed": c.Name}, func(w io.Writer) {
				fmt.Fprintf(w, "removed bridge %s; the chat app's bot and the hub node stay (delete them there if no longer wanted)\n", c.Name)
			})
		}),
	}
}
