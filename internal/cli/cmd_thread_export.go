package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/export"
)

// exportResult is what `thread export` says when it wrote a file.
type exportResult struct {
	Path     string `json:"path"`
	Messages int    `json:"messages"`
	Format   string `json:"format"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

// threadExportCommand writes a conversation to a file people can keep:
// Markdown to read, or JSON Lines in myco's own fields (internal/export).
func threadExportCommand() *Command {
	return &Command{
		Name:    "export",
		Summary: "Write a conversation to Markdown or JSON Lines",
		Long: "Markdown is for people (who said what, when, attachments by name); jsonl has one JSON object per message\n" +
			"in myco's own fields, for programs and backups. End-to-end encrypted messages this agent can read are\n" +
			"written decrypted: the file is plain text. Attachments are listed by name and size, not included.",
		Examples: []string{
			"myco thread export --id msg_abc123 --output chat.md",
			"myco thread export --id msg_abc123 --as jsonl --steps > chat.jsonl",
		},
		Mutating: true,
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Env: "MYCO_THREAD_ID", Help: "Thread ID (= ID of its first message)"},
			{Name: "as", Kind: Enum, Values: []string{"md", "jsonl"}, Default: "md", Help: "md = Markdown to read; jsonl = one JSON object per message (--format is how this command reports, as everywhere)"},
			{Name: "steps", Kind: Bool, Help: "Also write what the agent did for each task (its tools and on what), where this agent may read it"},
			{Name: "output", Kind: String, Default: "-", Help: "File to write ('-' = stdout)"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			format, out := e.in.String("as"), e.in.String("output")
			if format == "" {
				format = "md"
			}
			if out == "" {
				out = "-"
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			msgs, err := c.Thread(e.in.Ctx, e.in.String("id"))
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				path := out
				if out != "-" {
					if path, err = filepath.Abs(out); err != nil {
						return err
					}
				}
				return e.dryRun(exportResult{Path: path, Messages: len(msgs), Format: format, DryRun: true})
			}
			var steps map[string][]export.Step
			if e.in.Bool("steps") {
				steps = map[string][]export.Step{}
				skipped := 0
				for _, m := range msgs {
					if m.Type != "task" {
						continue
					}
					tr, err := c.Trace(e.in.Ctx, m.ID, 0)
					if err != nil {
						skipped++
						continue
					}
					if s := export.StepsOf(tr.Events); len(s) > 0 {
						steps[m.ID] = s
					}
				}
				if skipped > 0 {
					fmt.Fprintf(e.in.Stderr, "note: the steps of %d task(s) were left out (this agent may not read them, or the hub keeps none)\n", skipped)
				}
			}
			body, err := render(msgs, steps, format)
			if err != nil {
				return err
			}
			if out == "-" {
				_, err := e.in.Stdout.Write(body)
				return err
			}
			path, err := filepath.Abs(out)
			if err != nil {
				return err
			}
			// A conversation may hold what was end-to-end encrypted: only
			// this user reads the file.
			if err := os.WriteFile(path, body, 0o600); err != nil {
				return errorf("io_error", ExitError, "check the folder exists and can be written", "writing %s: %v", path, err)
			}
			res := exportResult{Path: path, Messages: len(msgs), Format: format}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintf(w, "wrote %d message(s) to %s (%s)\n", res.Messages, res.Path, res.Format)
			})
		}),
	}
}

func render(msgs []api.Message, steps map[string][]export.Step, format string) ([]byte, error) {
	if format == "jsonl" {
		return export.JSONL(msgs, steps)
	}
	return []byte(export.Markdown(msgs, steps, time.Now(), nil)), nil
}
