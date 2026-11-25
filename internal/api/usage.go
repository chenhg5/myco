package api

import "github.com/myco-ai/myco/internal/domain"

type UsageReport = domain.UsageReport
type UsageRow = domain.UsageRow
type UsageRequest struct {
	Source  string        `json:"source,omitempty"` // runner (default) or session
	Reports []UsageReport `json:"reports"`
}
type UsageRecorded struct {
	Recorded int `json:"recorded"`
}
type UsageAgent struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Owner       string `json:"owner"`
	Harness     string `json:"harness"`
	Machine     string `json:"machine"`
}
type UsageList struct {
	From   string       `json:"from"`
	To     string       `json:"to"`
	Rows   []UsageRow   `json:"rows"`
	Agents []UsageAgent `json:"agents"`
}
