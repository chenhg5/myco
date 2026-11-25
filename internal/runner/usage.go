package runner

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
)

// Optional so old/fake Hub implementations remain compatible.
type UsageUploader interface {
	PostUsage(context.Context, api.UsageRequest) (*api.UsageRecorded, error)
}
type runUsage struct {
	mu      sync.Mutex
	key     string
	reports []api.UsageReport
}

func (u *runUsage) add(ev harness.Event, model, project, message string) {
	if ev.Kind != "usage" {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.key == "" {
		u.key = "runner:" + rand.Text()
	}
	if ev.Model != "" {
		model = ev.Model
	}
	input := ev.InputTokens - ev.CacheReadTokens - ev.CacheWriteTokens
	if input < 0 {
		input = 0
	}
	u.reports = append(u.reports, api.UsageReport{Key: fmt.Sprintf("%s:%d", u.key, len(u.reports)), Day: time.Now().UTC().Format("2006-01-02"), Model: model, Project: project, MessageID: message, Calls: 1, InputTokens: input, CacheReadTokens: ev.CacheReadTokens, CacheWriteTokens: ev.CacheWriteTokens, OutputTokens: ev.OutputTokens, CostUSD: ev.CostUSD})
}
func (r *Runner) postUsage(ctx context.Context, u *runUsage) {
	if os.Getenv("MYCO_NO_USAGE") == "1" {
		return
	}
	up, ok := r.hub.(UsageUploader)
	if !ok {
		return
	}
	u.mu.Lock()
	reports := append([]api.UsageReport(nil), u.reports...)
	u.mu.Unlock()
	call, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	for len(reports) > 0 {
		n := min(100, len(reports))
		_, err := up.PostUsage(call, api.UsageRequest{Source: "runner", Reports: reports[:n]})
		if err != nil {
			r.log.Debug("numeric usage report unavailable")
			return
		}
		reports = reports[n:]
	}
}
