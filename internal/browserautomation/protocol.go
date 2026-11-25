// Package browserautomation owns the optional browser process and its small
// local protocol. go-rod and CDP remain in the separately released component.
package browserautomation

import "github.com/myco-ai/myco/internal/api"

const Protocol = 1

type ComponentStatus = api.BrowserComponentStatus

// ComponentError preserves the stable native protocol code and terminal reason.
// Native error text can contain page content and never crosses into a Hub job.
type ComponentError struct {
	Code    string
	Reason  string
	Host    string
	Applied *int
}

func (e *ComponentError) Error() string { return e.Code }
