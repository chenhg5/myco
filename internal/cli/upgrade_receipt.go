package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/runnerupdate"
)

// Run keeps an upgrade receipt outside command setup: invalid configuration or
// flags can fail before the upgrade handler is reached. Detached App upgrades
// must retain those failures too, rather than report that they are still running.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string, tty bool) (code int) {
	file := upgradeReceiptFile(args)
	if file == "" {
		return runCLI(ctx, args, stdin, stdout, stderr, getenv, tty)
	}
	out, errOut := &receiptBuffer{}, &receiptBuffer{}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	defer func() {
		panicValue := recover()
		receipt := runnerupdate.Receipt{State: "succeeded", At: time.Now().UTC()}
		if code != 0 || panicValue != nil {
			receipt.State = "failed"
			receipt.Error = "CLI upgrade failed; retry the update"
			if panicValue != nil {
				receipt.Error = "CLI upgrade ended unexpectedly"
			}
			raw := errOut.String()
			if i := strings.LastIndex(raw, "\n{"); i >= 0 {
				raw = raw[i+1:]
			}
			var e Error
			if json.Unmarshal([]byte(raw), &e) == nil && e.Message != "" {
				receipt.Error = runnerupdate.SafeMessage(e.Message)
			}
		} else {
			raw := bytes.TrimSpace(out.Bytes())
			if out.truncated {
				receipt.State = "failed"
				receipt.Error = "CLI upgrade result exceeded the receipt limit"
			} else if json.Valid(raw) {
				receipt.Result = append(json.RawMessage(nil), raw...)
				var v struct {
					Target string `json:"target"`
				}
				_ = json.Unmarshal(raw, &v)
				receipt.Target = v.Target
			} else if len(raw) > 0 {
				receipt.Result, _ = json.Marshal(map[string]string{"output": runnerupdate.SafeMessage(string(raw))})
			}
		}
		if err := runnerupdate.WriteFile(file, receipt); err != nil {
			if code == 0 {
				code = ExitError
			}
			b, _ := json.Marshal(Error{Code: "upgrade_receipt_failed", Message: "cannot save the private CLI upgrade receipt", Suggestion: "choose a writable --result-file and retry", Exit: ExitError})
			_, _ = fmt.Fprintln(stderr, string(b))
		}
		if panicValue != nil {
			panic(panicValue)
		}
	}()
	return runCLI(ctx, args, stdin, io.MultiWriter(stdout, out), io.MultiWriter(stderr, errOut), getenv, tty)
}

// Capture is bounded and never turns a truncated fragment into a JSON result.
type receiptBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *receiptBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (64 << 10) - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
func upgradeReceiptFile(args []string) string {
	for _, arg := range args {
		if arg == "--dry-run" || arg == "--dry-run=true" {
			return ""
		}
	}
	command := ""
	file := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--dry-run" {
			return ""
		}
		if strings.HasPrefix(arg, "--result-file=") {
			file = strings.TrimPrefix(arg, "--result-file=")
			continue
		}
		if arg == "--result-file" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				file = args[i]
			}
			continue
		}
		if command == "" && !strings.HasPrefix(arg, "-") {
			command = arg
			continue
		}
		if command == "" && (arg == "--config" || arg == "--hub" || arg == "--hub-name" || arg == "--format") {
			i++
		}
	}
	if command != "upgrade" {
		return ""
	}
	return file
}
