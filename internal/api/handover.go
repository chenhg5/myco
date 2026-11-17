package api

import "time"

// HandoverSide is one session of a handover.
type HandoverSide struct {
	Binding     Binding  `json:"binding"`
	MachineID   string   `json:"machine_id,omitempty"`
	MachineName string   `json:"machine_name,omitempty"`
	Runtime     *Runtime `json:"runtime,omitempty"`
}

// Handover is one change of the session behind a node (「换一个来接着做」):
// the name, conversations, projects and aliases stay; the session changes.
type Handover struct {
	ID    string    `json:"id"` // "hov_..."
	Agent string    `json:"agent"`
	At    time.Time `json:"at"`
	// By is who started it (a member's name, "admin" or an agent).
	By   string       `json:"by,omitempty"`
	Note string       `json:"note,omitempty"`
	From HandoverSide `json:"from"`
	// To is empty while State is waiting.
	To HandoverSide `json:"to"`
	// State: waiting (a link, not taken yet), done, expired.
	State  string     `json:"state"`
	DoneAt *time.Time `json:"done_at,omitempty"`
	// OpenTasks went back to the queue when it was taken.
	OpenTasks []string `json:"open_tasks"`
	// Back is the handover this one undid (a switch back), if any.
	Back      string    `json:"back,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// HandoverLinkRequest is the body of POST /v1/agents/{name}/handover-link.
type HandoverLinkRequest struct {
	Note string `json:"note,omitempty"`
	// Back switches back to the session that handover replaced: run the
	// command in that session.
	Back string `json:"back,omitempty"`
}

// HandoverLink is a one-time take-over link (24 hours). The token is only
// inside URL and the commands, shown once.
type HandoverLink struct {
	Handover   Handover `json:"handover"`
	URL        string   `json:"url,omitempty"`
	Command    string   `json:"command,omitempty"`     // curl -fsSL '<url>' | sh
	CommandPS1 string   `json:"command_ps1,omitempty"` // Windows PowerShell
	DryRun     bool     `json:"dry_run,omitempty"`
}

// HandoverRequest is the body of POST /v1/agents/{name}/handover: a new
// session on a machine takes the node over.
type HandoverRequest struct {
	Machine string `json:"machine"` // machine id or name
	CLI     string `json:"cli,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Model   string `json:"model,omitempty"`
	Access  string `json:"access,omitempty"`
	Note    string `json:"note,omitempty"`
	// Back switches back: the session that handover replaced resumes on
	// its machine (machine, cli and dir come from it).
	Back string `json:"back,omitempty"`
}

// HandoverResult answers POST /v1/agents/{name}/handover.
type HandoverResult struct {
	Handover Handover     `json:"handover"`
	Spawn    *SpawnResult `json:"spawn,omitempty"`
	DryRun   bool         `json:"dry_run,omitempty"`
}

// HandoverList answers GET /v1/agents/{name}/handovers.
type HandoverList struct {
	Handovers []Handover `json:"handovers"`
}

// TakeOverRequest is the body of POST /v1/takeover (no credential: the
// link token is one).
type TakeOverRequest struct {
	Token   string  `json:"token"`
	Binding Binding `json:"binding"`
}

// TakeOverResponse gives the session its identity and the handover note.
type TakeOverResponse struct {
	Agent    Agent    `json:"agent"`
	Token    string   `json:"token"`
	Handover Handover `json:"handover"`
	Brief    string   `json:"brief"`
}

// HandoverBrief answers GET /v1/me/handover.
type HandoverBrief struct {
	Handover Handover `json:"handover"`
	Brief    string   `json:"brief"`
}
