// Package remotedesktop defines the small, versioned contract with the optional
// remote desktop component. It contains no capture, codec or WebRTC dependency.
package remotedesktop

const Protocol = 1

type Description struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}
type Offer struct {
	Description Description `json:"description"`
	PairingCode string      `json:"pairing_code,omitempty"`
	Control     bool        `json:"control"`
}
type Device struct {
	Machine string `json:"machine"`
	Name    string `json:"name"`
	OS      string `json:"os"`
	Control bool   `json:"control"`
}
type Session struct {
	ID      string       `json:"id"`
	Machine string       `json:"machine"`
	State   string       `json:"state"`
	Offer   *Offer       `json:"offer,omitempty"`
	Answer  *Description `json:"answer,omitempty"`
	Error   string       `json:"error,omitempty"`
}
type ComponentStatus struct {
	Protocol      int    `json:"protocol"`
	Version       string `json:"version"`
	Installed     bool   `json:"installed"`
	Supported     bool   `json:"supported"`
	Running       bool   `json:"running"`
	PairingCode   string `json:"pairing_code,omitempty"`
	DownloadBytes int64  `json:"download_bytes,omitempty"`
	Reason        string `json:"reason,omitempty"`
}
