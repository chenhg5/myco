// Package browserautomation owns the optional browser process and its small
// local protocol. go-rod and CDP remain in the separately released component.
package browserautomation

import "github.com/myco-ai/myco/internal/api"

const Protocol = 1

type ComponentStatus = api.BrowserComponentStatus
