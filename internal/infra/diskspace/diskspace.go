// Package diskspace protects uploads with the available space of their volume.
package diskspace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/myco-ai/myco/internal/domain"
)

type Space struct{ Available, Total uint64 }

// Check reserves both an absolute floor and a percentage, including the
// pending write. Available is the space available to this process, not root.
func Check(s Space, incoming int64, minBytes int64, minPercent int) error {
	floor := uint64(minBytes)
	proportional := s.Total/100*uint64(minPercent) + (s.Total%100)*uint64(minPercent)/100
	if proportional > floor {
		floor = proportional
	}
	if incoming < 0 || s.Available < floor || uint64(incoming) > s.Available-floor {
		return domain.Errf(domain.CodeInvalidState, "Hub disk space is low: %d bytes available; reserve %d bytes plus %d bytes for this upload", s.Available, floor, incoming).WithSuggestion("free space on the Hub storage volume, then upload again")
	}
	return nil
}
func Guard(paths []string, minBytes int64, minPercent int) func(int64) error {
	return func(incoming int64) error {
		for _, path := range paths {
			p := path
			for {
				if _, err := os.Stat(p); err == nil {
					break
				}
				parent := filepath.Dir(p)
				if parent == p {
					return fmt.Errorf("cannot find storage volume for %s", path)
				}
				p = parent
			}
			s, err := Read(p)
			if err != nil {
				return fmt.Errorf("cannot check Hub storage volume: %w", err)
			}
			if err = Check(s, incoming, minBytes, minPercent); err != nil {
				return err
			}
		}
		return nil
	}
}
