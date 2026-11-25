package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/uispec"
)

// uiFlag adds a checked myco-ui block to a message body.
var uiFlag = Flag{Name: "ui", Kind: String,
	Help: "Append a myco-ui block (structured UI the app draws) from this JSON file ('-' = stdin); checked first, see myco ui schema"}

const uiSuggestion = "fix the reported errors, then run myco ui check --file <file>; myco ui schema --format text lists components, props and examples"

func uiSchemaCommand() *Command {
	return &Command{
		Name:    "schema",
		Summary: "Describe the components a myco-ui block may use (the guide as text, the catalog as JSON)",
		Long: `A myco-ui block is structured UI in a message: a fenced code block with the info string
myco-ui holding JSON (cards, tables, metrics, steps, charts, pictures, buttons...). The App draws
it; other clients show the JSON. With --format text this prints a guide for writing one; as JSON
it prints the catalog the App and myco ui check use.`,
		Examples: []string{
			"myco ui schema --format text",
			"myco ui schema --format json",
		},
		Run: handler(func(e *env) error {
			return e.p.Print(json.RawMessage(uispec.CatalogJSON()), func(w io.Writer) {
				io.WriteString(w, uispec.Guide())
			})
		}),
	}
}

func uiCheckCommand() *Command {
	return &Command{
		Name:    "check",
		Summary: "Check a myco-ui block's JSON before sending it",
		Long: `Reads the JSON inside a myco-ui block (without the fence), in the nested or the flat form, and
checks it against the catalog: known components, required props and their types, children, and
the size limits. Exit 0 when it is good; exit 2 with error "invalid_ui" and one detail per
problem otherwise. myco message reply/progress/send --ui run the same check.`,
		Examples: []string{
			"myco ui check --file block.json",
			"cat block.json | myco ui check --file -",
		},
		Flags: []Flag{{Name: "file", Kind: String, Required: true, Help: "The block's JSON ('-' = stdin)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("file"); err != nil {
				return err
			}
			data, err := e.readUIFile("file")
			if err != nil {
				return err
			}
			spec, ps := uispec.Check(data)
			if len(ps) > 0 {
				return uiProblemsError(ps, "")
			}
			n := spec.Len()
			return e.p.Print(map[string]any{"ok": true, "elements": n}, func(w io.Writer) {
				fmt.Fprintf(w, "ok: a valid myco-ui block (%d elements)\n", n)
			})
		}),
	}
}

// uiProblemsError is the error for a block that does not pass the check.
func uiProblemsError(ps []uispec.Problem, flag string) *Error {
	what := "the myco-ui block"
	if flag != "" {
		what = "the myco-ui block in --" + flag
	}
	noun := "problems"
	if len(ps) == 1 {
		noun = "problem"
	}
	details := make([]ErrorDetail, len(ps))
	for i, p := range ps {
		details[i] = ErrorDetail{Path: p.Path, Message: p.Message}
	}
	return &Error{Code: "invalid_ui", Message: fmt.Sprintf("%s has %d %s; first: %s", what, len(ps), noun, ps[0]),
		Suggestion: uiSuggestion, Details: details, Exit: ExitUsage}
}

// readUIFile reads the JSON file a flag names ('-' = stdin).
func (e *env) readUIFile(flag string) ([]byte, error) {
	path := strings.TrimSpace(e.in.String(flag))
	limit := int64(uispec.Catalog().Limits.MaxBytes) * 4
	if path == "-" {
		return io.ReadAll(io.LimitReader(e.in.Stdin, limit))
	}
	if err := safePath(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errorf("not_found", ExitNotFound, "check the path", "cannot read %s: %v", path, err)
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// messageBody reads a message body from bodyFlag/fileFlag and appends the
// --ui block, if one is given. With --ui the text is optional (the body is
// then just the block). The block is checked here, before anything is sent.
func (e *env) messageBody(bodyFlag, fileFlag string) (string, error) {
	ui := strings.TrimSpace(e.in.String("ui"))
	if ui == "" {
		return e.readBody(bodyFlag, fileFlag)
	}
	if ui == "-" && e.in.String(fileFlag) == "-" {
		return "", errorf("invalid_usage", ExitUsage, "read one of them from a file", "--%s and --ui cannot both read stdin", fileFlag)
	}
	body := ""
	if e.in.IsSet(bodyFlag) || strings.TrimSpace(e.in.String(fileFlag)) != "" {
		b, err := e.readBody(bodyFlag, fileFlag)
		if err != nil {
			return "", err
		}
		body = b
	}
	data, err := e.readUIFile("ui")
	if err != nil {
		return "", err
	}
	return appendUIBlock(body, data)
}

// appendUIBlock checks a block's JSON and appends it, fenced, to body.
func appendUIBlock(body string, data []byte) (string, error) {
	if _, ps := uispec.Check(data); len(ps) > 0 {
		return "", uiProblemsError(ps, "ui")
	}
	fence := uispec.Fence(data)
	body = strings.TrimRight(body, " \t\r\n")
	if body == "" {
		return fence, nil
	}
	return body + "\n\n" + fence, nil
}
