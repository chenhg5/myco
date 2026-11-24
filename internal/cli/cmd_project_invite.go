package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
)

func projectInviteCommand() *Command {
	return &Command{
		Name: "invite-agent", Summary: "Invite one ordinary agent to your project (coordinators only; asks the owner when approval is required)", Mutating: true,
		Flags: []Flag{{Name: "name", Kind: String, Required: true, Help: "Project name"}, {Name: "display-name", Kind: String, Help: "Pre-filled display name"}, {Name: "description", Kind: String, Help: "Pre-filled description"}, {Name: "note", Kind: String, Help: "Role note in this project (up to 200 characters)"}, {Name: "wait", Kind: Duration, Help: "Wait for the owner's approval, e.g. 10m"}, {Name: "request", Kind: String, Help: "Retrieve an earlier invitation by its ID (shown only once)"}},
		Run: handler(func(e *env) error {
			wait := e.in.Duration("wait")
			if wait < 0 {
				return errorf("invalid_argument", ExitUsage, "use a positive duration such as --wait 10m", "negative --wait")
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			var out *api.ProjectInviteResponse
			name, id := e.in.String("name"), e.in.String("request")
			dry := e.in.Bool("dry-run")
			if id != "" {
				out, err = c.ProjectInvitation(e.in.Ctx, name, id, false)
			} else {
				out, err = c.InviteProjectAgent(e.in.Ctx, name, api.ProjectInviteRequest{DisplayName: e.in.String("display-name"), Description: e.in.String("description"), Note: e.in.String("note")}, dry)
			}
			if err != nil {
				return projectInviteError(err)
			}
			if dry {
				return e.dryRun(out)
			}

			ctx := e.in.Ctx
			var cancel context.CancelFunc
			if wait > 0 {
				ctx, cancel = context.WithTimeout(ctx, wait)
				defer cancel()
			}
			for out.Status == "pending" && wait > 0 {
				select {
				case <-ctx.Done():
					return e.p.Print(out, func(w io.Writer) {
						fmt.Fprintf(w, "Waiting for the project owner. Resume with: myco project invite-agent --name %s --request %s --wait 10m\n", name, out.ID)
					})
				case <-time.After(time.Second):
				}
				next, err := c.ProjectInvitation(ctx, name, out.ID, false)
				if err != nil {
					if ctx.Err() != nil {
						break
					}
					return projectInviteError(err)
				}
				out = next
			}
			if out.Status == "approved" {
				out, err = c.ProjectInvitation(e.in.Ctx, name, out.ID, true)
				if err != nil {
					return projectInviteError(err)
				}
			}
			return e.p.Print(out, func(w io.Writer) {
				if out.Sentence != "" {
					fmt.Fprintln(w, out.Sentence)
					return
				}
				switch out.Status {
				case "pending":
					fmt.Fprintf(w, "Invitation %s awaits the project owner's approval. Ask them to approve the card, or enable coordinator invitations in project settings.\nRetrieve with: myco project invite-agent --name %s --request %s --wait 10m\n", out.ID, name, out.ID)
				case "ready":
					fmt.Fprintln(w, "This invitation link was already shown once. If it was lost, request a fresh invitation.")
				default:
					fmt.Fprintf(w, "Invitation %s: %s\n", out.ID, out.Status)
				}
			})
		}),
	}
}
func projectInviteError(err error) error {
	if ce, ok := client.AsError(err); ok && ce.Status == 404 {
		return errorf("unsupported", ExitUsage, "upgrade the workspace Hub to use project agent invitations", "project invitation endpoint not found (or request no longer exists)")
	}
	return err
}
