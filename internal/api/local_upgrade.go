package api

import (
	"encoding/json"
	"time"
)

// LocalIdleRestartRequest is a credential-free request owned by one local
// runner. It is not sent to the Hub or accepted from a remote machine.
type LocalIdleRestartRequest struct {
	ID          string    `json:"id"`
	Target      string    `json:"target"`
	RequestedAt time.Time `json:"requested_at"`
	Deadline    time.Time `json:"deadline"`
}
type LocalRunnerRuntime struct {
	PID       int       `json:"pid"`
	Instance  string    `json:"instance"`
	Version   string    `json:"version"`
	At        time.Time `json:"at"`
	RequestID string    `json:"request_id,omitempty"`
	State     string    `json:"state,omitempty"`
	Error     string    `json:"error,omitempty"`
}
type CLIUpgradeReceipt struct {
	ID     string          `json:"id,omitempty"`
	State  string          `json:"state"`
	Target string          `json:"target,omitempty"`
	At     time.Time       `json:"at"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// LocalUpgradeStatus is GET/POST /app/local/upgrade and
// POST /app/local/upgrade/restart (App-local routes, not Hub endpoints).
type LocalUpgradeStatus struct {
	CLI         *CLIUpgradeReceipt       `json:"cli,omitempty"`
	Installed   string                   `json:"installed,omitempty"`
	Runner      *LocalRunnerRuntime      `json:"runner,omitempty"`
	Restart     *LocalIdleRestartRequest `json:"restart,omitempty"`
	Pending     bool                     `json:"pending"`
	Expired     bool                     `json:"expired"`
	Unsupported bool                     `json:"unsupported"`
	Error       string                   `json:"error,omitempty"`
}
