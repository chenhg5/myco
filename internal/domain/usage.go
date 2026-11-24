package domain

import (
	"context"
	"time"
)

// UsageReport contains numeric CLI accounting, never prompts or output.
type UsageReport struct {
	Key              string  `json:"key,omitempty"`
	Day              string  `json:"day"`
	Model            string  `json:"model,omitempty"`
	Project          string  `json:"project,omitempty"`
	MessageID        string  `json:"message_id,omitempty"`
	Calls            int64   `json:"calls"`
	InputTokens      int64   `json:"input_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}
type UsageRow struct {
	Day              string  `json:"day"`
	Agent            string  `json:"agent"`
	Project          string  `json:"project"`
	Model            string  `json:"model"`
	Source           string  `json:"source"`
	Calls            int64   `json:"calls"`
	InputTokens      int64   `json:"input_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}
type UsageFilter struct {
	From, To, Project string
	Owner             string
}
type UsageRepository interface {
	Record(context.Context, string, string, []UsageReport, time.Time) (int, error)
	List(context.Context, UsageFilter) ([]UsageRow, error)
}
