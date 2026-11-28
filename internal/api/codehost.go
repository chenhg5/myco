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

// CodeHostQueryFilters narrows a read-only provider query before pagination.
// Clients negotiate forge.filters before sending this optional object.
type CodeHostQueryFilters struct {
	Author       string `json:"author,omitempty"`
	Label        string `json:"label,omitempty"`
	UpdatedAfter string `json:"updated_after,omitempty"`
	Draft        string `json:"draft,omitempty"`
	Sort         string `json:"sort,omitempty"`
}

// CodeHostCommit contains only repository history metadata, never credentials.
type CodeHostCommit struct {
	SHA     string   `json:"sha"`
	Parents []string `json:"parents"`
	Subject string   `json:"subject"`
	Message string   `json:"message"`
	Author  string   `json:"author"`
	Date    string   `json:"date"`
	URL     string   `json:"url"`
}
type CodeHostBranch struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}
