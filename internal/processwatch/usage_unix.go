//go:build unix

package processwatch

import (
	"context"
	"errors"
	"github.com/myco-ai/myco/internal/oscap"
)

func readUsage(ctx context.Context) (usageSnapshot, error) {
	v, e := oscap.Call(ctx, nil, "system.usage", nil)
	if e != nil {
		return usageSnapshot{}, e
	}
	u, ok := v.(oscap.Usage)
	if !ok {
		return usageSnapshot{}, errors.New("invalid usage sample")
	}
	return usageSnapshot{CPU: u.CPU, Total: u.MemoryTotal, Available: u.MemoryAvailable, Load: u.Load}, nil
}
