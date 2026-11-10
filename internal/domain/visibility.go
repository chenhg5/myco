package domain

// Visibility says who can find an agent and give it work.
type Visibility string

const (
	// VisibilityTeam: everyone on the hub (allow_from still applies).
	VisibilityTeam Visibility = "team"
	// VisibilityPrivate: only its owner and the owner's own agents; the
	// hub's owner and admins still see it (to manage it) but cannot send.
	VisibilityPrivate Visibility = "private"
)

// ParseVisibility checks v ("" means team).
func ParseVisibility(v string) (Visibility, error) {
	switch Visibility(v) {
	case "", VisibilityTeam:
		return VisibilityTeam, nil
	case VisibilityPrivate:
		return VisibilityPrivate, nil
	}
	return "", Errf(CodeInvalidArgument, "invalid visibility %q", v).
		WithSuggestion("use team (everyone on the hub) or private (only its owner)")
}

// Private reports whether only the agent's owner may find and use it. An
// agent without an owner (a hub without members) is never private.
func (a *Agent) Private() bool { return a.Visibility == VisibilityPrivate && a.Owner != "" }

// Viewer is whoever looks at or writes to agents: the hub's managers see
// everything; a member, their own; an agent, its owner's.
type Viewer struct {
	All    bool   // the admin token, owners and admins, hubs without members
	Member string // a member's id (the app)
	Agent  *Agent // an agent's own token
}

// Sees reports whether v may find a (in lists and lookups).
func (v Viewer) Sees(a *Agent) bool {
	if v.All || !a.Private() {
		return true
	}
	if v.Agent != nil {
		return v.Agent.Name == a.Name || (v.Agent.Owner != "" && v.Agent.Owner == a.Owner)
	}
	return v.Member != "" && v.Member == a.Owner
}

// MaySendTo reports whether sender may give a private agent tasks and notes:
// only its owner (from the app) and the owner's own agents. Senders without
// an owner (the hub's admin token, hubs without members) may.
func MaySendTo(sender, to *Agent) bool {
	if !to.Private() || sender.Name == to.Name {
		return true
	}
	return sender.Owner == "" || sender.Owner == to.Owner
}
