package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

// pinCommands keep things at hand in a conversation (置顶): a web address,
// a file or a message, shown above the conversation in the App.
func pinCommands() *Command {
	adminFile := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (people; agents use their own)"}
	with := Flag{Name: "with", Kind: String, Required: true, Help: "The conversation's other party: the agent, or the person's sender (e.g. @console:ann)"}
	side := Flag{Name: "a", Kind: String, Help: "People only: the conversation's first party, to look at two agents' conversation (default: you)"}
	pinFlag := Flag{Name: "pin", Kind: String, Required: true, Help: "Pin id (pin_...)"}
	add := &Command{
		Name:     "add",
		Summary:  "Pin a link, a file or a message to a conversation",
		Mutating: true,
		Long: `Pins something to your conversation with --with: the App shows it above the conversation, one
click away, for both of you (a design doc, the preview's address, the message with the plan).
Exactly one of --url, --file, --file-id, --message. --file uploads a local file first.
Pinning the same thing again only changes its title. A conversation keeps 20 pins at most.`,
		Examples: []string{
			`myco pin add --with @console:ann --url https://example.com/spec --title 设计稿`,
			`myco pin add --with @console:ann --file report.pdf`,
			`myco pin add --with @console:ann --message msg_abc123 --title 上线计划`,
		},
		Flags: []Flag{
			with,
			{Name: "url", Kind: String, Help: "A web address (http or https), e.g. a page published with myco page"},
			{Name: "file", Kind: String, Help: "A local file to upload and pin"},
			{Name: "file-id", Kind: String, Help: "A file already on the hub (f_...), e.g. one attached to a message"},
			{Name: "message", Kind: String, Help: "A message in the conversation (msg_...)"},
			{Name: "title", Kind: String, Help: "What it is called on the pin (default: the address, file name or message id)"},
			side, adminFile,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("with"); err != nil {
				return err
			}
			req := api.PinCreate{A: e.in.String("a"), With: e.in.String("with"), Title: e.in.String("title")}
			given := 0
			for _, f := range []string{"url", "file", "file-id", "message"} {
				if e.in.String(f) != "" {
					given++
				}
			}
			if given != 1 {
				return errorf("invalid_argument", ExitUsage, "pin one thing: --url, --file, --file-id or --message", "give exactly one of --url, --file, --file-id, --message")
			}
			switch {
			case e.in.String("url") != "":
				req.Kind, req.Target = "link", strings.TrimSpace(e.in.String("url"))
			case e.in.String("message") != "":
				req.Kind, req.Target = "message", strings.TrimSpace(e.in.String("message"))
			default:
				req.Kind = "file"
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "pin_add", "with": req.With, "kind": req.Kind, "target": req.Target,
					"file": e.in.String("file"), "file_id": e.in.String("file-id"), "title": req.Title})
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			if req.Kind == "file" {
				var f *api.File
				if path := e.in.String("file"); path != "" {
					f, err = c.UploadFile(e.in.Ctx, path)
				} else {
					f, err = c.FileMeta(e.in.Ctx, e.in.String("file-id"))
				}
				if err != nil {
					return err
				}
				req.File = &api.Attachment{FileID: f.ID, Name: f.Name, Size: f.Size, SHA256: f.SHA256, ContentType: f.ContentType}
			}
			p, err := c.AddPin(e.in.Ctx, req)
			if err != nil {
				return pinErr(err)
			}
			return e.p.Print(p, func(w io.Writer) {
				fmt.Fprintf(w, "pinned %s (%s) to the conversation with %s\n", p.Title, p.ID, req.With)
			})
		}),
	}
	list := &Command{
		Name:     "list",
		Summary:  "List a conversation's pins",
		Examples: []string{"myco pin list --with @console:ann"},
		Flags:    []Flag{with, side, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("with"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			l, err := c.Pins(e.in.Ctx, e.in.String("a"), e.in.String("with"))
			if err != nil {
				return pinErr(err)
			}
			return e.p.Print(l, func(w io.Writer) {
				if len(l.Pins) == 0 {
					fmt.Fprintln(w, "no pins")
					return
				}
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tKIND\tTITLE\tTARGET\tBY")
				for _, p := range l.Pins {
					target := p.Target
					if p.File != nil {
						target = p.File.FileID
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.ID, p.Kind, p.Title, target, p.AddedBy)
				}
				tw.Flush()
			})
		}),
	}
	rename := &Command{
		Name:     "rename",
		Summary:  "Give a pin another title",
		Mutating: true,
		Examples: []string{"myco pin rename --with @console:ann --pin pin_abc --title 设计稿 v2"},
		Flags:    []Flag{with, pinFlag, {Name: "title", Kind: String, Required: true, Help: "The new title"}, side, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("with", "pin", "title"); err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "pin_rename", "with": e.in.String("with"), "pin": e.in.String("pin"), "title": e.in.String("title")})
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			p, err := c.RenamePin(e.in.Ctx, e.in.String("pin"), api.PinUpdate{A: e.in.String("a"), With: e.in.String("with"), Title: e.in.String("title")})
			if err != nil {
				return pinErr(err)
			}
			return e.p.Print(p, func(w io.Writer) { fmt.Fprintf(w, "renamed %s to %s\n", p.ID, p.Title) })
		}),
	}
	rm := &Command{
		Name:     "rm",
		Summary:  "Unpin one",
		Mutating: true,
		Examples: []string{"myco pin rm --with @console:ann --pin pin_abc"},
		Flags:    []Flag{with, pinFlag, side, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("with", "pin"); err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "pin_rm", "with": e.in.String("with"), "pin": e.in.String("pin")})
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			if err := c.RemovePin(e.in.Ctx, e.in.String("a"), e.in.String("with"), e.in.String("pin")); err != nil {
				return pinErr(err)
			}
			return e.p.Print(map[string]any{"removed": e.in.String("pin")}, func(w io.Writer) { fmt.Fprintf(w, "unpinned %s\n", e.in.String("pin")) })
		}),
	}
	return (&Command{Name: "pin", Summary: "Keep links, files and messages at hand above a conversation (置顶)"}).Add(add, list, rename, rm)
}

// pinErr says what to do on a hub without pins.
func pinErr(err error) error {
	if e, ok := client.AsError(err); ok && e.Status == 404 && strings.Contains(e.Message, "does not keep pins") {
		return errorf("not_supported", ExitNotFound, "the hub needs v0.20.3 or later (sudo myco upgrade on the server); until then, write the link in your reply", "this hub has no pins yet")
	}
	// An older hub has no /v1/pins at all.
	if e, ok := client.AsError(err); ok && e.Status == 404 && e.Code == "" {
		return errorf("not_supported", ExitNotFound, "the hub needs v0.20.3 or later (sudo myco upgrade on the server); until then, write the link in your reply", "this hub has no pins yet")
	}
	return err
}
