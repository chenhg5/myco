package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/client"
)

const waitAgainHint = "处理完后（或现在）再运行 `myco inbox wait`。"

// inboxWaitTimeout uses an explicit flag first, then MYCO_WAIT_TIMEOUT,
// then the current agent's harness. A binding read is only needed when the
// local config does not already say which harness this agent uses.
func inboxWaitTimeout(e *env, c *client.Client) (time.Duration, string, string, error) {
	harness := e.cfg.Harness
	if harness == "" && c != nil {
		if me, err := c.Me(e.in.Ctx); err == nil {
			harness = me.Binding.Harness
		}
	}
	if harness == "" && e.in.Getenv != nil {
		harness = detectBinding(e.in.Getenv).Harness
	}
	label := harness
	if harness == "claude" || harness == "claude-cloud" {
		label = "Claude Code"
	}
	if label == "" {
		label = "unknown harness"
	}
	raw := ""
	if e.in.IsSet("timeout") {
		raw = e.in.String("timeout")
	} else if e.in.Getenv != nil {
		raw = e.in.Getenv("MYCO_WAIT_TIMEOUT")
	}
	if raw == "" {
		if harness == "claude" || harness == "claude-cloud" {
			raw = "55m"
		} else {
			raw = "10m"
		}
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, "", "", errorf("invalid_argument", ExitUsage, "use a positive duration such as 10m or 55m", "invalid inbox wait timeout %q", raw)
	}
	return d, raw, label, nil
}

func inboxWaitHint(taskHint string) string {
	if taskHint == "" {
		return waitAgainHint
	}
	return strings.TrimSpace(taskHint) + " " + waitAgainHint
}

func appendInboxWaitHint(err *Error) {
	if strings.Contains(err.Suggestion, waitAgainHint) {
		return
	}
	if err.Suggestion == "" {
		err.Suggestion = waitAgainHint
	} else {
		err.Suggestion = fmt.Sprintf("%s；%s", err.Suggestion, waitAgainHint)
	}
}
