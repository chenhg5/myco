package runner

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/harness"
)

// timedRun keeps attempts separate. Error text from a failed invocation must
// not become the successful first-word sample of a later retry or steer.
func (r *Runner) timedRun(ctx context.Context, req harness.Request, log *slog.Logger, start time.Time, invocation int) (harness.Result, error) {
	mark := func(stage string) {
		log.Info("run latency", "stage", stage, "invocation", invocation, "elapsed_ms", time.Since(start).Milliseconds(), "observed_at_ms", time.Now().UnixMilli())
	}
	nextEvent, nextTiming := req.OnEvent, req.OnTiming
	var firstText sync.Once
	req.OnEvent = func(ev harness.Event) {
		if ev.Text != "" && (ev.Kind == "text" || ev.Kind == "delta" && ev.Stream == "text") {
			firstText.Do(func() { mark("first_text") })
		}
		if nextEvent != nil {
			nextEvent(ev)
		}
	}
	req.OnTiming = func(stage string) {
		mark(stage)
		if nextTiming != nil {
			nextTiming(stage)
		}
	}
	mark("harness_called")
	res, err := r.harness.Run(ctx, req)
	log.Info("run latency", "stage", "harness_returned", "invocation", invocation, "succeeded", err == nil, "elapsed_ms", time.Since(start).Milliseconds(), "observed_at_ms", time.Now().UnixMilli())
	return res, err
}
