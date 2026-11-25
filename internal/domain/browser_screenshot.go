package domain

import (
	"context"
	"time"
)

// BrowserScreenshot is a private artifact belonging to a browser action.
type BrowserScreenshot struct {
	ID, AgentID, OwnerID, MediaType string
	Bytes                           int
	ExpiresAt                       time.Time
	Data                            []byte
}

type BrowserScreenshotRepository interface {
	Save(context.Context, BrowserScreenshot) error
	Get(context.Context, string, string, string, time.Time) (*BrowserScreenshot, error)
	List(context.Context, string, time.Time) ([]BrowserScreenshot, error)
	Prune(context.Context, time.Time) error
}
