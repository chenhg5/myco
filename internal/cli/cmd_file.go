package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

var attachFlag = Flag{Name: "attach", Kind: List,
	Help: "Attach a local file (uploaded first) or an already uploaded file id f_...; repeatable"}

// resolveAttachments uploads local paths and passes file ids through.
func (e *env) resolveAttachments(c *client.Client, dry bool) ([]string, error) {
	var ids []string
	for _, a := range e.in.List("attach") {
		if strings.HasPrefix(a, "f_") {
			if _, err := os.Stat(a); err != nil { // not a local file named f_...
				ids = append(ids, a)
				continue
			}
		}
		if err := safePath(a); err != nil {
			return nil, err
		}
		st, err := os.Stat(a)
		if err != nil {
			return nil, errorf("not_found", ExitNotFound, "check the path", "cannot attach %s: %v", a, err)
		}
		if dry {
			fmt.Fprintf(e.in.Stderr, "would upload %s (%d bytes)\n", a, st.Size())
			continue
		}
		f, err := c.UploadFile(e.in.Ctx, a)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(e.in.Stderr, "uploaded %s as %s (%d bytes)\n", a, f.ID, f.Size)
		ids = append(ids, f.ID)
	}
	return ids, nil
}

func fileCommands() *Command {
	upload := &Command{
		Name:     "upload",
		Summary:  "Upload a file; attach the returned id to messages with --attach f_...",
		Mutating: true,
		Examples: []string{"myco file upload --path report.md", "myco message send --to coder --body \"see report\" --attach report.md   # uploads for you"},
		Flags:    []Flag{{Name: "path", Kind: String, Required: true, Help: "Local file"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("path"); err != nil {
				return err
			}
			path := e.in.String("path")
			if err := safePath(path); err != nil {
				return err
			}
			st, err := os.Stat(path)
			if err != nil {
				return errorf("not_found", ExitNotFound, "check the path", "cannot read %s: %v", path, err)
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "upload", "path": path, "size": st.Size()})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			f, err := c.UploadFile(e.in.Ctx, path)
			if err != nil {
				return err
			}
			return e.p.Print(f, func(w io.Writer) {
				fmt.Fprintf(w, "uploaded %s as %s (%d bytes, sha256 %s…)\n", f.Name, f.ID, f.Size, f.SHA256[:12])
			})
		}),
	}
	get := &Command{
		Name:    "get",
		Summary: "Download an attachment you sent or received (checksum verified)",
		Examples: []string{
			"myco file get --id f_abc123                  # into the current directory",
			"myco file get --id f_abc123 --output /tmp/x.md",
		},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "File id (from a message's attachments)"},
			{Name: "output", Kind: String, Default: ".", Help: "Directory or file path to write"},
			{Name: "force", Kind: Bool, Help: "Overwrite an existing file"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			out := e.in.String("output")
			if st, err := os.Stat(out); err == nil && !st.IsDir() && !e.in.Bool("force") {
				return errorf("conflict", ExitConflict, "pass --force to overwrite, or choose another --output", "%s already exists", out)
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			f, path, err := c.DownloadFile(e.in.Ctx, e.in.String("id"), out)
			if err != nil {
				return err
			}
			return e.p.Print(map[string]any{"file": f, "path": path}, func(w io.Writer) {
				fmt.Fprintf(w, "saved %s (%d bytes, checksum ok)\n", path, f.Size)
			})
		}),
	}
	return (&Command{Name: "file", Summary: "Upload and download message attachments"}).Add(upload, get)
}

func printAttachments(w io.Writer, atts []api.Attachment) {
	for _, a := range atts {
		fmt.Fprintf(w, "attachment: %s  %s  %d bytes  (myco file get --id %s)\n", a.FileID, a.Name, a.Size, a.FileID)
	}
}
