package cli

import (
	"bufio"
	"context"
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
	g := &Command{Name: "im", Summary: "Chat-app bots (Feishu/Lark, Slack): one per agent, to talk to it from the chat app",
		Long: `A bridge is an agent's bot in a chat app: what you write to it becomes a task
for that agent, its progress updates one message in place, and its answer
comes back in the same chat. While a task is open, what you write is added to
it, and answers its question. Each agent has its own bot (as voice is per
agent).

The bot speaks as a node of yours on the hub (im-<name>, receiving through its
inbox). It connects out to the chat app (Feishu long connection, Slack Socket
Mode): no public address is needed. This machine's runner service serves the
bridges configured here; myco im serve runs them in the foreground.`}
	g.Add(imConnectCommand(), imServeCommand(), imListCommand(), imAllowCommand(), imSetCommand(), imClaimCommand(), imRemoveCommand())
	return g
}

func (e *env) imDir() string { return im.Dir(config.Dir(e.defaultPath)) }

// BridgeName is the default name of an agent's bot on a platform:
// "feishu-coder", cut to fit and made of what a bridge name may hold.
func BridgeName(platform, agent string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(agent) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	n := strings.Trim(platform+"-"+b.String(), "-")
	if len(n) > 32 {
		n = strings.TrimRight(n[:32], "-")
	}
	return n
}

var imPlatforms = []string{"feishu", "lark", "slack"}

func imConnectCommand() *Command {
	return &Command{Name: "connect", Summary: "Create or connect a bot: Feishu/Lark by scanning a QR code, Slack from an app manifest", Mutating: true,
		Args: "<feishu|lark|slack>",
		Examples: []string{
			"myco im connect feishu --agent coder            # coder's bot: scan the QR code with the Feishu app",
			"myco im connect feishu --app-id cli_xxx --app-secret-file secret.txt --agent coder",
			"myco im connect slack --agent coder             # prints the link that creates the Slack app",
			"myco im connect slack --bot-token-file bot.txt --app-token-file app.txt --agent coder",
		},
		Flags: []Flag{
			{Name: "agent", Kind: String, Required: true, Help: "The agent this bot is for: what is written to the bot goes to it"},
			{Name: "name", Kind: String, Help: "Bridge name (default: <platform>-<agent>), 1–32 lowercase letters, digits or hyphens"},
			{Name: "project", Kind: String, Help: "Put new conversations in this project"},
			{Name: "lang", Kind: Enum, Values: []string{"zh", "en"}, Help: "What the bot speaks (default: zh for Feishu, en otherwise)"},
			{Name: "app-id", Kind: String, Help: "Feishu/Lark: an existing app's App ID (skips the QR code)"},
			{Name: "app-secret-file", Kind: String, Help: "Feishu/Lark: file with that app's App Secret"},
			{Name: "bot-token-file", Kind: String, Env: "MYCO_SLACK_BOT_TOKEN_FILE", Help: "Slack: file with the Bot User OAuth Token (xoxb-…)"},
			{Name: "app-token-file", Kind: String, Env: "MYCO_SLACK_APP_TOKEN_FILE", Help: "Slack: file with the app-level token (xapp-…, scope connections:write)"},
			{Name: "owner", Kind: String, Help: "Feishu/Lark: the owner's open_id when already known (the App passes it after its own scan)"},
			{Name: "qr-png", Kind: String, Help: "Feishu/Lark: also save the QR code as a PNG here"},
			{Name: "timeout", Kind: Duration, Default: "10m", Help: "Feishu/Lark: how long to wait for the scan"},
			{Name: "profile", Kind: String, Help: "Speak as this existing agent profile instead of a new im-<name> node"},
			{Name: "llm-base-url", Kind: String, Help: "Front desk: an OpenAI-compatible model's base URL (it answers as the agent, as in voice chats)"},
			{Name: "llm-model", Kind: String, Help: "Front desk: the model (a fast one)"},
			{Name: "llm-key-file", Kind: String, Help: "Front desk: file with the model's API key"},
		},
		Run: handler(func(e *env) error {
			if len(e.in.Args) != 1 {
				return errorf("invalid_usage", ExitUsage, "myco im connect feishu (or lark, slack)", "name one platform")
			}
			platform := strings.ToLower(e.in.Args[0])
			if !slices.Contains(imPlatforms, platform) {
				return errorf("invalid_argument", ExitUsage, "use feishu, lark or slack", "unknown platform %q", platform)
			}
			if err := e.in.Require("agent"); err != nil {
				return err
			}
			name := e.in.String("name")
			if name == "" {
				name = BridgeName(platform, e.in.String("agent"))
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
			if err := e.imDesk(cfg); err != nil {
				return err
			}
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
					cfg.AppID, cfg.AppSecret, cfg.Owner = id, secret, e.in.String("owner")
				} else if dry {
					return e.dryRun(map[string]any{"action": "im connect", "platform": platform, "name": name,
						"steps": []string{"show a QR code to scan with the Feishu/Lark app", "create node im-" + name, "save " + filepath.Join(dir, name+".json")}})
				} else {
					reg, err := e.scanFeishu(ctx, platform == "lark", e.in.String("qr-png"), e.in.Duration("timeout"))
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
						"1) open "+imslack.CreateURL(e.in.String("agent"))+" and create the app (the manifest is filled in); "+
							"2) install it to your workspace and copy the Bot User OAuth Token (OAuth & Permissions); "+
							"3) under Basic Information, create an app-level token with connections:write; "+
							"4) run again with --bot-token-file and --app-token-file (or paste them when asked)",
						"Slack tokens are needed")
				}
				cfg.BotToken, cfg.AppToken = bot, app
				check = imslack.New(bot, app, nil).Check
			}
			if cfg.Owner == "" {
				cfg.NewClaim(time.Now())
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
				fmt.Fprintf(w, "connected %s bot %q for %s as bridge %s\n", platform, botName, cfg.Agent, name)
				if cfg.Claim != "" {
					fmt.Fprintf(w, "to become its owner, send this to the bot in a direct message: /claim %s\n", cfg.Claim)
				}
				fmt.Fprintln(w, "this machine's runner service starts it within 30 seconds; or run: myco im serve --name "+name)
			})
		}),
	}
}

// scanFeishu shows the registration QR code and waits for the scan.
func (e *env) scanFeishu(ctx context.Context, lark bool, png string, timeout time.Duration) (*feishu.Registration, error) {
	p, err := (&feishu.Registrar{Lark: lark}).Begin(ctx)
	if err != nil {
		return nil, errorf("unavailable", ExitUnavailable, "check that this machine reaches accounts.feishu.cn, or pass --app-id with --app-secret-file", "Feishu registration: %v", err)
	}
	w := e.in.Stderr
	if lark {
		fmt.Fprintln(w, "Scan with the Lark phone app to create the bot:")
	} else {
		fmt.Fprintln(w, "用飞书手机 App 扫码创建机器人 (scan with the Feishu app):")
	}
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

// imDesk applies the --llm-* flags (the front desk's model).
func (e *env) imDesk(c *im.Config) error {
	if e.in.IsSet("llm-base-url") {
		c.LLMBaseURL = e.in.String("llm-base-url")
	}
	if e.in.IsSet("llm-model") {
		c.LLMModel = e.in.String("llm-model")
	}
	if f := e.in.String("llm-key-file"); f != "" {
		k, err := readSecretFile(f, "--llm-key-file")
		if err != nil {
			return err
		}
		c.LLMKey = k
	}
	if e.in.IsSet("llm-model") || e.in.IsSet("llm-key-file") {
		c.NoDesk = false
	}
	if (c.LLMModel == "") != (c.LLMKey == "") {
		return errorf("invalid_usage", ExitUsage, "pass both --llm-model and --llm-key-file (and --llm-base-url unless it is OpenAI)", "the front desk needs a model and its key")
	}
	return nil
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
		Examples: []string{"myco im serve", "myco im serve --name feishu-coder"},
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

// serveBridges runs the bridges configured on this machine: new ones start,
// changed ones restart and removed ones stop within a few seconds (the
// runner service calls it, so the App and myco im need no restart).
func (e *env) serveBridges(ctx context.Context, log *slog.Logger) {
	type running struct {
		mod    time.Time
		cancel context.CancelFunc
	}
	dir := e.imDir()
	live := map[string]*running{}
	modOf := func(n string) time.Time {
		st, err := os.Stat(filepath.Join(dir, n+".json"))
		if err != nil {
			return time.Time{}
		}
		return st.ModTime()
	}
	start := func(n string, mod time.Time) {
		bctx, cancel := context.WithCancel(ctx)
		live[n] = &running{mod: mod, cancel: cancel}
		go func() {
			for bctx.Err() == nil {
				err := e.runBridge(bctx, n, log)
				if bctx.Err() != nil {
					return
				}
				log.Error("bridge stopped; restarting in a minute", "bridge", n, "err", err)
				select {
				case <-bctx.Done():
					return
				case <-time.After(time.Minute):
				}
			}
		}()
	}
	scan := func() {
		names, err := im.List(dir)
		if err != nil {
			log.Warn("reading bridges failed", "err", err)
			return
		}
		seen := map[string]bool{}
		for _, n := range names {
			seen[n] = true
			mod := modOf(n)
			if r := live[n]; r != nil {
				if r.mod.Equal(mod) {
					continue
				}
				r.cancel() // its config changed: start it again
				log.Info("bridge settings changed; restarting it", "bridge", n)
			}
			start(n, mod)
		}
		for n, r := range live {
			if !seen[n] {
				r.cancel()
				delete(live, n)
				log.Info("bridge removed; stopped it", "bridge", n)
			}
		}
	}
	scan()
	t := time.NewTicker(5 * time.Second)
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
				c, err := im.Load(dir, n)
				if err != nil {
					continue
				}
				r := c.Redacted()
				if pc, err := config.Load(c.Profile); err == nil && pc.Token != "" {
					r["hub"], r["node"] = pc.Hub, pc.Agent
					if cl, err := client.New(pc.Hub, pc.Token); err == nil {
						cctx, cancel := context.WithTimeout(e.in.Ctx, 5*time.Second)
						if me, err := cl.Me(cctx); err == nil {
							r["online"] = me.Online
						}
						cancel()
					}
				}
				out = append(out, r)
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
		Examples: []string{"myco im allow --name feishu-coder --user ou_123", "myco im allow --name slack-coder --user U0123 --remove"},
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
				fmt.Fprintf(w, "%s allows: owner %s %s (a running bridge picks this up within seconds)\n", c.Name, orDash(c.Owner), strings.Join(c.AllowFrom, " "))
			})
		}),
	}
}

func imSetCommand() *Command {
	return &Command{Name: "set", Summary: "Change a bridge's project, language, or front desk (model and instructions)", Mutating: true,
		Examples: []string{"myco im set --name feishu-coder --project website"},
		Flags: []Flag{{Name: "name", Kind: String, Required: true, Help: "Bridge name"},
			{Name: "project", Kind: String, Help: "Project for new conversations (\"\" for none)"},
			{Name: "lang", Kind: Enum, Values: []string{"zh", "en"}, Help: "What the bot speaks"},
			{Name: "llm-base-url", Kind: String, Help: "Front desk: an OpenAI-compatible model's base URL (it answers as the agent, as in voice chats)"},
			{Name: "llm-model", Kind: String, Help: "Front desk: the model (a fast one)"},
			{Name: "llm-key-file", Kind: String, Help: "Front desk: file with the model's API key"},
			{Name: "no-desk", Kind: Bool, Help: "Turn the front desk off: messages go straight to the agent"},
			{Name: "prompt-file", Kind: String, Help: "Front desk: file with your own instructions for this bot (rules for whoever talks to it); an empty file clears them"}},
		Run: handler(func(e *env) error {
			c, err := e.loadBridge()
			if err != nil {
				return err
			}
			if f := e.in.String("prompt-file"); f != "" {
				b, err := os.ReadFile(f)
				if err != nil {
					return errorf("not_found", ExitNotFound, "", "reading --prompt-file: %v", err)
				}
				c.Prompt = strings.TrimSpace(string(b))
				if n := len([]rune(c.Prompt)); n > im.MaxPrompt {
					return errorf("invalid_argument", ExitUsage, "shorten it", "the instructions are %d characters; at most %d", n, im.MaxPrompt)
				}
			}
			if e.in.Bool("no-desk") {
				c.LLMBaseURL, c.LLMModel, c.LLMKey, c.NoDesk = "", "", "", true
			} else if err := e.imDesk(c); err != nil {
				return err
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
				fmt.Fprintf(w, "%s: project=%s lang=%s desk=%s (a running bridge picks this up within seconds)\n", c.Name, orDash(c.Project), orDash(c.Lang), orDash(c.LLMModel))
			})
		}),
	}
}

func imClaimCommand() *Command {
	return &Command{Name: "claim", Summary: "Make a new claim code for a bot that has no owner yet (the old one stops working)", Mutating: true,
		Long: `A bot connected without a QR scan (Slack, or Feishu with --app-id) gets its
owner by "/claim <code>" sent to it in a direct message. A code lasts 24 hours
and allows 5 wrong tries; this makes a new one.`,
		Examples: []string{"myco im claim --name slack-coder"},
		Flags:    []Flag{{Name: "name", Kind: String, Required: true, Help: "Bridge name"}},
		Run: handler(func(e *env) error {
			c, err := e.loadBridge()
			if err != nil {
				return err
			}
			if c.Owner != "" {
				return errorf("conflict", ExitConflict, "remove the bridge and connect it again to change its owner", "bridge %s already has an owner", c.Name)
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "im claim", "bridge": c.Name})
			}
			code := c.NewClaim(time.Now())
			if err := im.Save(e.imDir(), c); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"name": c.Name, "claim": code, "expires_at": c.ClaimExpires}, func(w io.Writer) {
				fmt.Fprintf(w, "send this to the bot in a direct message within 24 hours: /claim %s\n", code)
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
