package runner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/client"
)

// finishContext gives delivery its own budget after the harness has stopped.
// A cancelled run must not cancel the attempt to report its result.
func (r *Runner) finishContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), r.finishTimeout)
}

// retryFinish retries only transport and server failures. The second return
// value reports whether a retryable failure exhausted the delivery budget.
func (r *Runner) retryFinish(ctx context.Context, log *slog.Logger, action string, fn func(context.Context) error) (error, bool) {
	delay := r.finishRetryMin
	for {
		err := fn(ctx)
		if err == nil {
			return nil, false
		}
		if ctx.Err() != nil {
			return err, true
		}
		if !retryableFinishError(err) {
			return err, false
		}
		deadline, _ := ctx.Deadline()
		pause := min(delay, time.Until(deadline))
		if pause <= 0 {
			return err, true
		}
		log.Warn("hub unavailable while delivering result; retrying", "action", action, "err", err, "retry_in", pause)
		timer := time.NewTimer(pause)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return err, true
		}
		delay = min(delay*2, 16*time.Second)
	}
}

func retryableFinishError(err error) bool {
	if e, ok := client.AsError(err); ok {
		if e.Status >= 400 && e.Status < 500 {
			return false
		}
		return e.Retryable || e.Status >= 500 || e.Code == client.CodeUnavailable || e.Code == client.CodeTimeout
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

var resultSecret = regexp.MustCompile(`(?i)myco_[a-z0-9_-]+|sk-[a-z0-9_-]+|bearer\s+\S+|(?:token|secret|api[_-]?key|password)\s*[:=]\s*\S+`)

func resultPrefix(output string) string {
	const limit = 512
	if len(output) > limit {
		output = output[:limit] + "…"
	}
	return resultSecret.ReplaceAllString(strings.ToValidUTF8(output, "�"), "[redacted]")
}

func (r *Runner) logFinishFailure(log *slog.Logger, action string, err error, exhausted bool, output string) {
	if exhausted {
		log.Error("result not delivered to hub after retries", "action", action, "err", err, "output_prefix", resultPrefix(output))
		return
	}
	log.Error("finishing message failed", "action", action, "err", err)
}
