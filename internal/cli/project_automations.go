package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
)

func (e *env) automationClient() (*client.Client, bool, error) {
	if e.in.Getenv(config.EnvAdminToken) == "" && e.in.String("admin-token-file") == "" {
		if c, err := e.agentClient(); err == nil {
			if e.in.String("project") == "" {
				return nil, true, errorf("permission_denied", ExitPermission, "pass --project <name>; ask the project owner to enable coordinator scheduling or approve the request", "agents manage automations through their project coordinator role")
			}
			return c, true, nil
		}
	}
	c, err := e.adminClient()
	return c, false, err
}
func automationUpgradeError(err error) error {
	if err == nil {
		return nil
	}
	if ce, ok := client.AsError(err); ok {
		if (ce.Status == 404 && (strings.Contains(ce.Message, "404 page not found") || strings.Contains(ce.Message, "no such route"))) || strings.Contains(ce.Message, `unknown field "starts_at"`) || strings.Contains(ce.Message, `unknown field "ends_at"`) || strings.Contains(ce.Message, `unknown field "once"`) {
			return errorf("unsupported", ExitUsage, "upgrade the workspace Hub for project scheduling, date bounds and once", "this Hub does not support this automation operation (or the automation no longer exists)")
		}
	}
	return err
}
func (e *env) automationBounds(req *api.AutomationRequest) error {
	for _, v := range []struct {
		flag string
		dst  **string
	}{{"starts", &req.StartsAt}, {"ends", &req.EndsAt}} {
		if !e.in.IsSet(v.flag) {
			continue
		}
		value := e.in.String(v.flag)
		if value != "" {
			if day, err := time.Parse("2006-01-02", value); err == nil {
				if v.flag == "ends" {
					day = day.AddDate(0, 0, 1)
				}
				value = day.Format(time.RFC3339)
			} else if _, err = time.Parse(time.RFC3339, value); err != nil {
				return errorf("invalid_argument", ExitUsage, "use YYYY-MM-DD (UTC days) or an RFC3339 timestamp", "invalid --%s date", v.flag)
			}
		}
		*v.dst = &value
	}
	if e.in.IsSet("once") {
		req.Once = boolPtr(e.in.Bool("once"))
	}
	return nil
}
func (e *env) changeProjectAutomation(c *client.Client, action, id string, req api.AutomationRequest) error {
	out, err := c.ChangeProjectAutomation(e.in.Ctx, e.in.String("project"), action, id, req)
	if err != nil {
		return automationUpgradeError(err)
	}
	return e.p.Print(out, func(w io.Writer) {
		if out.Status == "pending" {
			fmt.Fprintf(w, "Request %s awaits the project owner's approval. Ask them to approve the card, or enable coordinator scheduling in project settings.\n", out.ApprovalID)
		} else if out.Automation != nil {
			printAutomation(w, *out.Automation, "")
		} else {
			fmt.Fprintf(w, "removed %s\n", out.Removed)
		}
	})
}
