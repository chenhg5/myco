package runner

import "github.com/myco-ai/myco/internal/client"

// IsCredentialRejected reports a permanent authentication failure. The Hub
// also returns this after a machine and its nodes have been removed.
func IsCredentialRejected(err error) bool {
	e, ok := client.AsError(err)
	return ok && (e.Status == 401 || e.Status == 403)
}

func rejectedHubCredential(err error) bool { return IsCredentialRejected(err) }

// IsReplaced reports that another session took this node over (a
// handover): its old session must stop, for good.
func IsReplaced(err error) bool {
	e, ok := client.AsError(err)
	return ok && e.Code == "replaced"
}
