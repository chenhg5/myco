package api

// CredentialDescriptor is an application's requested read-only HTTP grant.
// It contains no provider business types or raw credentials.
type CredentialPath struct {
	Path   string `json:"path"`
	Prefix bool   `json:"prefix,omitempty"`
}
type CredentialDescriptor struct {
	Name           string           `json:"name"`
	Origin         string           `json:"origin"`
	Authentication string           `json:"authentication"`
	Paths          []CredentialPath `json:"paths"`
}
type CredentialConnection struct {
	ID string `json:"id"`
	CredentialDescriptor
	Source string `json:"source"`
}

// CredentialSource is discovered by the host, never specified by the iframe.
type CredentialSource struct {
	ID       string `json:"id"`
	CLI      string `json:"cli"`
	Host     string `json:"host"`
	Login    string `json:"login"`
	Origin   string `json:"origin"`
	BasePath string `json:"base_path"`
}
type CredentialConfirmation struct {
	Confirmation string `json:"confirmation"`
	CredentialDescriptor
	PrivateNetwork bool               `json:"private_network"`
	Verified       bool               `json:"verified"`
	Sources        []CredentialSource `json:"sources"`
}
type CredentialConnect struct {
	Confirmation string `json:"confirmation"`
	Token        string `json:"token"`
	Source       string `json:"source,omitempty"`
	AllowPrivate bool   `json:"allow_private"`
}
