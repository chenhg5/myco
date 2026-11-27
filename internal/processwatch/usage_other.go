//go:build !unix && !windows

package processwatch

import (
	"context"
	"errors"
)

func readUsage(context.Context) (usageSnapshot, error) {
	return usageSnapshot{}, errors.New("machine usage unsupported")
}
