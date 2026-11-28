package api

// CodeHostConfirmation is returned only to the trusted desktop owner dialog.
// It is not a page bridge or an agent action response.
type CodeHostConfirmation struct {
	Confirmation   string `json:"confirmation"`
	Provider       string `json:"provider"`
	Host           string `json:"host"`
	Source         string `json:"source"`
	PrivateNetwork bool   `json:"private_network"`
}

// CodeHostConnect is sent from the trusted App after a local person confirms.
// The token is never sent to the application iframe, page records or audit log.
type CodeHostConnect struct {
	Confirmation string `json:"confirmation"`
	Token        string `json:"token"`
	AllowPrivate bool   `json:"allow_private"`
}
