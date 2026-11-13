package cli

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
)

// resumeTokenAndHub accepts a raw credential or a whole /r/ URL. Never put
// the raw input in an error: it is a credential.
func resumeTokenAndHub(raw string) (token, hub string, err error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, parseErr := url.Parse(raw)
		if parseErr == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
			i := strings.LastIndex(u.Path, "/r/")
			if i >= 0 {
				token, hub = u.Path[i+3:], u.Scheme+"://"+u.Host+u.Path[:i]
			}
		}
	} else {
		token = raw
	}
	if !strings.HasPrefix(token, "myco_resume_") || strings.ContainsAny(token, "/?# \t\r\n") {
		return "", "", errorf("invalid_argument", ExitUsage, "pass a myco_resume_ token or its /r/ URL", "invalid resume credential")
	}
	return token, hub, nil
}

func (e *env) resumeBinding() api.Binding {
	if b := detectBinding(e.in.Getenv); b.Harness != "" && b.SessionID != "" {
		return b
	}
	wd, _ := os.Getwd()
	if id, _, ok := findCodexSession(wd, false, time.Now()); ok {
		return api.Binding{Harness: "codex", SessionID: id, Environment: hostname(), Workdir: wd}
	}
	return inboxBinding(wd)
}

func (e *env) resumeIdentity(raw, hub string, dry bool) (*api.RegisterResponse, error) {
	token, fromURL, err := resumeTokenAndHub(raw)
	if err != nil {
		return nil, err
	}
	if hub == "" {
		hub = fromURL
	}
	if hub == "" {
		hub, err = e.hubURL()
		if err != nil {
			return nil, err
		}
	}
	b := e.resumeBinding()
	if dry {
		return &api.RegisterResponse{Agent: api.Agent{Binding: b}, DryRun: true}, nil
	}
	c, err := client.New(hub, "")
	if err != nil {
		return nil, err
	}
	res, err := c.Resume(e.in.Ctx, api.ResumeRequest{Token: token, Binding: b})
	if err != nil {
		return nil, err
	}
	if b.Harness == "inbox" && !e.explicitConfig {
		// An inbox-only session has no harness environment variable with which
		// the next invocation could find its per-session profile.
		e.cfgPath, e.explicitConfig = e.defaultPath, true
	}
	if err := e.saveIdentity(hub, res.Agent.Name, res.Token, res.Agent.Binding); err != nil {
		return nil, fmt.Errorf("agent resumed but saving config failed: %w", err)
	}
	e.cfg.ResumeToken = token
	if err := config.Save(e.cfgPath, e.cfg); err != nil {
		return nil, fmt.Errorf("agent resumed but saving recovery credential failed: %w", err)
	}
	return res, nil
}

func (e *env) autoResume() error {
	raw := e.in.Getenv("MYCO_RESUME")
	if raw == "" || e.cfg.Token != "" {
		return nil
	}
	res, err := e.resumeIdentity(raw, "", false)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.in.Stderr, "myco: resumed as %s; credentials saved to %s\n", res.Agent.Name, e.cfgPath)
	return nil
}

func resumeCommands() []*Command {
	resume := &Command{Name: "resume", Summary: "Restore this agent's identity in a new sandbox", Mutating: true,
		Examples: []string{"myco agent resume --token myco_resume_... --hub https://hub.example.com", "myco agent resume --token https://hub.example.com/r/myco_resume_..."},
		Flags:    []Flag{{Name: "token", Kind: String, Help: "Recovery token or full /r/ URL; keep private (default: MYCO_RESUME, which keeps it out of the process list)"}},
		Run: handler(func(e *env) error {
			token := e.in.String("token")
			if token == "" {
				token = strings.TrimSpace(e.in.Getenv("MYCO_RESUME"))
			}
			if token == "" {
				return errorf("invalid_argument", ExitUsage, "pass --token, or set MYCO_RESUME to the token or the /r/ link", "no recovery token given")
			}
			hub := ""
			if e.in.IsSet("hub") {
				hub = e.in.String("hub")
			}
			if hub == "" {
				_, hub, _ = resumeTokenAndHub(token)
			}
			if hub == "" {
				hub = e.cfg.Hub
			}
			res, err := e.resumeIdentity(token, hub, e.in.Bool("dry-run"))
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "resume", "binding": res.Agent.Binding, "hub": hub})
			}
			out := map[string]any{"agent": res.Agent, "config": e.cfgPath}
			if res.Agent.Binding.Harness == "inbox" || strings.HasSuffix(res.Agent.Binding.Harness, "-cloud") {
				out["receive"] = "keep myco inbox wait --timeout 55m running in the background; restart it when it exits"
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "resumed as %s; credentials saved to %s\n", res.Agent.Name, e.cfgPath)
				if v, ok := out["receive"].(string); ok {
					fmt.Fprintln(w, v)
				}
			})
		})}
	link := &Command{Name: "resume-link", Summary: "Rotate or revoke your agent's recovery link", Mutating: true,
		Long:     "Generating a new link invalidates the old one. Keep it outside temporary sandboxes and never share it.",
		Examples: []string{"myco agent resume-link", "myco agent resume-link --revoke", "myco agent resume-link --dry-run"},
		Flags:    []Flag{{Name: "revoke", Kind: Bool, Help: "Invalidate the current link"}},
		Run: handler(func(e *env) error {
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "resume_link", "revoke": e.in.Bool("revoke")})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			if e.in.Bool("revoke") {
				if err := c.RevokeResume(e.in.Ctx, ""); err != nil {
					return upgradeResumeError(err)
				}
				e.cfg.ResumeToken = ""
				if err := config.Save(e.cfgPath, e.cfg); err != nil {
					return err
				}
				return e.p.Print(map[string]any{"revoked": true}, func(w io.Writer) { fmt.Fprintln(w, "recovery link revoked") })
			}
			res, err := c.CreateResume(e.in.Ctx, "", false)
			if err != nil {
				return upgradeResumeError(err)
			}
			e.cfg.ResumeToken = res.Token
			if err := config.Save(e.cfgPath, e.cfg); err != nil {
				return err
			}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintln(w, "Previous recovery link is invalid. Save this command outside the sandbox and keep it private:\n  "+res.Command)
			})
		})}
	return []*Command{resume, link}
}

func upgradeResumeError(err error) error {
	var ce *client.Error
	if errors.As(err, &ce) && ce.Status == 404 {
		return errorf("not_found", ExitNotFound, "upgrade the Hub to use recovery links", "this Hub does not support agent recovery links")
	}
	return err
}

// ensureResume creates only the first recovery link. Re-running join never
// silently revokes a link the agent saved outside this sandbox.
func (e *env) ensureResume(hub, agentToken string) string {
	c, err := client.New(hub, agentToken)
	if err != nil {
		return ""
	}
	link, err := c.CreateResume(e.in.Ctx, "", true)
	if err != nil {
		return ""
	} // old hubs and temporary outages keep join working
	if link.Token != "" {
		e.cfg.ResumeToken = link.Token
		if err := config.Save(e.cfgPath, e.cfg); err != nil {
			return ""
		}
		return link.Command
	}
	if e.cfg.ResumeToken != "" {
		return "curl -fsSL " + strings.TrimRight(hub, "/") + "/r/" + e.cfg.ResumeToken + " | sh"
	}
	return ""
}
