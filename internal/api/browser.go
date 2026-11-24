package api

// BrowserComponentStatus describes an optional local desktop component, not a
// Hub action provider. Protocol 1 deliberately advertises no agent actions.
type BrowserComponentStatus struct {
	Protocol        int    `json:"protocol"`
	Version         string `json:"version"`
	Installed       bool   `json:"installed"`
	Supported       bool   `json:"supported"`
	Running         bool   `json:"running"`
	UpdateAvailable bool   `json:"update_available,omitempty"`
	Reason          string `json:"reason,omitempty"`
	DownloadBytes   int64  `json:"download_bytes,omitempty"`
}
type BrowserProfile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Agents  bool   `json:"agents"`
}
type BrowserStatus struct {
	Protocol  int              `json:"protocol"`
	Version   string           `json:"version"`
	Installed bool             `json:"installed"`
	Supported bool             `json:"supported"`
	Reason    string           `json:"reason,omitempty"`
	Browsers  []string         `json:"browsers"`
	Machine   string           `json:"machine"`
	Profiles  []BrowserProfile `json:"profiles"`
}
