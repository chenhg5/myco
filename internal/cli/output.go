package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

// Exit codes are a stable contract (documented in `myco --help`).
const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitNotFound    = 3
	ExitPermission  = 4
	ExitConflict    = 5
	ExitUnavailable = 6
	ExitTimeout     = 7
	ExitDryRun      = 10
)

const exitCodeHelp = `Exit codes: 0 ok, 1 error, 2 invalid usage, 3 not found, 4 permission/auth,
5 conflict or invalid state, 6 hub unavailable (retryable), 7 wait timed out, 10 dry-run ok.
Output: JSON on stdout when not a TTY (or --format json); logs and errors on stderr.`

// Error is the CLI's structured error (printed as JSON to stderr).
type Error struct {
	Code       string         `json:"error"`
	Message    string         `json:"message"`
	Suggestion string         `json:"suggestion,omitempty"`
	Retryable  bool           `json:"retryable"`
	Command    string         `json:"command,omitempty"`
	Input      map[string]any `json:"input,omitempty"`
	Exit       int            `json:"exit_code"`
}

func (e *Error) Error() string { return e.Message }

// dryRunDone signals a successful dry-run (exit 10).
type dryRunDone struct{}

func (dryRunDone) Error() string { return "dry run" }

// exitStatus ends a command with a specific exit code and no error output
// (the command already printed what it had), e.g. a remote command's status.
type exitStatus int

func (s exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(s)) }

var exitByCode = map[string]int{
	"invalid_argument":     ExitUsage,
	"not_found":            ExitNotFound,
	"permission_denied":    ExitPermission,
	"unauthenticated":      ExitPermission,
	"conflict":             ExitConflict,
	"invalid_state":        ExitConflict,
	client.CodeUnavailable: ExitUnavailable,
	client.CodeTimeout:     ExitTimeout,
	"not_configured":       ExitUsage,
}

// toError normalizes any error into a CLI Error.
func toError(err error, cmd string) *Error {
	var ce *Error
	if errors.As(err, &ce) {
		if ce.Command == "" {
			ce.Command = cmd
		}
		return ce
	}
	var ue *cmdkit.UsageError
	if errors.As(err, &ue) {
		return &Error{Code: "invalid_usage", Message: ue.Message, Suggestion: ue.Suggestion, Command: ue.Command, Exit: ExitUsage}
	}
	if ae, ok := client.AsError(err); ok {
		exit, ok := exitByCode[ae.Code]
		if !ok {
			exit = ExitError
		}
		return &Error{Code: ae.Code, Message: ae.Message, Suggestion: ae.Suggestion, Retryable: ae.Retryable, Command: cmd, Exit: exit}
	}
	return &Error{Code: "error", Message: err.Error(), Command: cmd, Exit: ExitError}
}

func errorf(code string, exit int, suggestion, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Suggestion: suggestion, Exit: exit}
}

// Format is the output format.
type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

// Printer writes command results in the selected format.
type Printer struct {
	Format Format
	Out    io.Writer
	Err    io.Writer
}

// Print writes v as JSON, or calls text for human output.
func (p *Printer) Print(v any, text func(w io.Writer)) error {
	if p.Format == FormatJSON || text == nil {
		enc := json.NewEncoder(p.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	text(p.Out)
	return nil
}

// PrintError writes err to stderr (JSON when non-TTY/json, text otherwise).
func (p *Printer) PrintError(e *Error) {
	if p.Format == FormatJSON {
		b, _ := json.Marshal(e)
		fmt.Fprintln(p.Err, string(b))
		return
	}
	fmt.Fprintf(p.Err, "error: %s\n", e.Message)
	if e.Suggestion != "" {
		fmt.Fprintf(p.Err, "hint: %s\n", e.Suggestion)
	}
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
