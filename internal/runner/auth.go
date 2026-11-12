package runner

import "github.com/chenhg5/myco/internal/client"

// IsCredentialRejected reports a permanent authentication failure. The Hub
// also returns this after a machine and its nodes have been removed.
func IsCredentialRejected(err error) bool {
	e, ok := client.AsError(err)
	return ok && (e.Status == 401 || e.Status == 403)
}

func rejectedHubCredential(err error) bool { return IsCredentialRejected(err) }
