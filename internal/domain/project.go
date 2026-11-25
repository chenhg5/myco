package domain

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Project is a piece of work several agents (and people) do together: a
// goal, a brief every member can read, shared documents and links, and its
// members, some of them coordinators.
type Project struct {
	ID    string
	Name  string // short address: lowercase letters, digits, - and _
	Title string
	Goal  string
	// Brief is the shared background (Markdown): repository, rules, how to
	// test, what to watch out for.
	Brief      string
	Links      []ProjectLink
	Visibility ProjectVisibility
	Owner      string // member id of whoever created it ("" on hubs without members)
	Archived   bool
	// Machines are the computers (machine ids) its coordinators may start
	// agent sessions on: a permission of its own, set by its owner, apart
	// from the terminals it has (v0.21.2).
	Machines []string
	// CoordinatorInvites allows coordinators to invite ordinary agents without approval.
	CoordinatorInvites     bool
	CoordinatorAutomations bool
	// Zero storage overrides inherit the Hub defaults.
	FilesMax int
	BytesMax int64
	// Groups are the groups its members are sorted into (e.g. frontend,
	// backend), in display order; each member is in at most one.
	Groups    []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProjectLink points at material kept elsewhere (a repository, a document).
type ProjectLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// ProjectVisibility: who can see a project and its documents.
type ProjectVisibility string

const (
	// ProjectMembers: the people involved (its owner, the owners of its member
	// nodes), its member nodes, and the hub's owner and admins.
	ProjectMembers ProjectVisibility = "members"
	// ProjectTeam: everyone on the hub.
	ProjectTeam ProjectVisibility = "team"
)

// ProjectRole is a member node's role in a project.
type ProjectRole string

const (
	ProjectRoleMember ProjectRole = "member"
	// ProjectRoleCoordinator may edit the project and its member nodes' cards, and
	// add nodes of its own owner.
	ProjectRoleCoordinator ProjectRole = "coordinator"
)

// ProjectMember is a node in a project.
type ProjectMember struct {
	Project string // project id
	Agent   AgentName
	Role    ProjectRole
	// Note says what it does in this project.
	Note string
	// Group is its group in this project (one of Project.Groups); "" is
	// ungrouped.
	Group   string
	AddedBy string // who added it (member or agent name, for display)
	AddedAt time.Time
}

// ProjectDoc is a shared file of a project: a document or any uploaded
// file. Text (Markdown and the like, up to 64 KB) is kept in Body so agents
// read it with the project; anything else is a blob.
type ProjectDoc struct {
	Project     string
	Name        string // a file name, e.g. design.md or 设计稿.pdf; older documents have none (design)
	Body        string
	ContentType string // "" for documents written as text
	Size        int64
	Blob        string // blob id when the content is not in Body
	UpdatedBy   string
	UpdatedAt   time.Time
}

// IsBlob reports whether the content is in the blob store.
func (d *ProjectDoc) IsBlob() bool { return d.Blob != "" }

const (
	maxProjectTitle = 80
	maxProjectGoal  = 500
	// MaxProjectBrief is the brief's length in characters (it all goes in
	// every member's prompt); before v0.21.10 it was 8 KB.
	MaxProjectBrief = 16000
	maxProjectLinks = 20
	maxProjectNote  = 200
	// MaxProjectGroups bounds a project's groups, MaxGroupName a group's
	// name (characters).
	MaxProjectGroups = 30
	MaxGroupName     = 40
	maxProjectDoc    = 64 << 10
	MaxProjectDocs   = 100
	// MaxProjectFile bounds an uploaded file.
	MaxProjectFile = 20 << 20
	// MaxProjectText is the largest text file kept inline (read by agents
	// with the project).
	MaxProjectText = maxProjectDoc
)

// ParseFileName checks the name of a project file: a plain file name (no
// directories), kept as given.
func ParseFileName(s string) (string, error) {
	s = strings.TrimSpace(s)
	bad := s == "" || utf8.RuneCountInString(s) > 120 || strings.HasPrefix(s, ".") || strings.ContainsAny(s, "/\\:*?\"<>|")
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			bad = true
		}
	}
	if bad {
		return "", Errf(CodeInvalidArgument, "invalid file name %q", s).
			WithSuggestion("use a plain file name such as design.md or notes.pdf (no folders, not starting with a dot)")
	}
	return s, nil
}

// ParseProjectName checks a project's short name.
func ParseProjectName(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 40 || !namePattern.MatchString(s) {
		return "", Errf(CodeInvalidArgument, "invalid project name %q", s).
			WithSuggestion("use a short lowercase name such as myco or ios-app (letters, digits, - and _)")
	}
	return s, nil
}

// ParseDocName checks a document's name.
func ParseDocName(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(s, ".md")))
	if s == "" || len(s) > 60 || !namePattern.MatchString(s) {
		return "", Errf(CodeInvalidArgument, "invalid document name %q", s).
			WithSuggestion("use a short lowercase name such as design or conventions")
	}
	return s, nil
}

// ParseProjectVisibility checks v ("" means members).
func ParseProjectVisibility(v string) (ProjectVisibility, error) {
	switch ProjectVisibility(v) {
	case "", ProjectMembers:
		return ProjectMembers, nil
	case ProjectTeam:
		return ProjectTeam, nil
	}
	return "", Errf(CodeInvalidArgument, "invalid project visibility %q", v).
		WithSuggestion("use members (the people and nodes involved) or team (everyone on the hub)")
}

// ParseProjectRole checks r ("" means member).
func ParseProjectRole(r string) (ProjectRole, error) {
	switch ProjectRole(r) {
	case "", ProjectRoleMember:
		return ProjectRoleMember, nil
	case ProjectRoleCoordinator:
		return ProjectRoleCoordinator, nil
	}
	return "", Errf(CodeInvalidArgument, "invalid role %q", r).WithSuggestion("use member or coordinator")
}

// ProjectEdit changes a project; nil fields stay as they are.
type ProjectEdit struct {
	Title, Goal, Brief *string
	Links              *[]ProjectLink
}

// Apply validates and applies e.
func (p *Project) Apply(e ProjectEdit, now time.Time) error {
	if e.Title != nil {
		t := strings.TrimSpace(*e.Title)
		if utf8.RuneCountInString(t) > maxProjectTitle || strings.ContainsAny(t, "\n\r") {
			return Errf(CodeInvalidArgument, "a project title is one line of at most %d characters", maxProjectTitle)
		}
		p.Title = t
	}
	if e.Goal != nil {
		g := strings.TrimSpace(*e.Goal)
		if utf8.RuneCountInString(g) > maxProjectGoal {
			return Errf(CodeInvalidArgument, "a project goal is at most %d characters", maxProjectGoal)
		}
		p.Goal = g
	}
	if e.Brief != nil {
		if utf8.RuneCountInString(*e.Brief) > MaxProjectBrief || len(*e.Brief) > maxProjectDoc {
			return Errf(CodeInvalidArgument, "the brief is longer than %d characters", MaxProjectBrief).
				WithSuggestion("keep the brief short and put longer material in a project document")
		}
		p.Brief = strings.TrimSpace(*e.Brief)
	}
	if e.Links != nil {
		if len(*e.Links) > maxProjectLinks {
			return Errf(CodeInvalidArgument, "at most %d links", maxProjectLinks)
		}
		links := make([]ProjectLink, 0, len(*e.Links))
		for _, l := range *e.Links {
			l.Title, l.URL = strings.TrimSpace(l.Title), strings.TrimSpace(l.URL)
			if u, err := url.Parse(l.URL); err != nil || l.URL == "" || u.Scheme == "" {
				return Errf(CodeInvalidArgument, "invalid link %q", l.URL).WithSuggestion("use a full address such as https://github.com/owner/repo")
			}
			if utf8.RuneCountInString(l.Title) > maxProjectTitle {
				return Errf(CodeInvalidArgument, "a link title is at most %d characters", maxProjectTitle)
			}
			links = append(links, l)
		}
		p.Links = links
	}
	p.UpdatedAt = now
	return nil
}

// CheckNote validates a member's note.
func CheckNote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxProjectNote || strings.ContainsAny(s, "\n\r") {
		return "", Errf(CodeInvalidArgument, "a note is one line of at most %d characters", maxProjectNote)
	}
	return s, nil
}

// ParseGroupName checks a group's name: one trimmed line of at most
// MaxGroupName characters.
func ParseGroupName(s string) (string, error) {
	s = strings.TrimSpace(s)
	bad := s == "" || utf8.RuneCountInString(s) > MaxGroupName
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			bad = true
		}
	}
	if bad {
		return "", Errf(CodeInvalidArgument, "invalid group name %q", s).
			WithSuggestion("use a short name of at most 40 characters, such as frontend or 后端")
	}
	return s, nil
}

// ParseProjectGroups checks a project's full, ordered list of groups: each
// name valid, no name twice, at most MaxProjectGroups.
func ParseProjectGroups(list []string) ([]string, error) {
	if len(list) > MaxProjectGroups {
		return nil, Errf(CodeInvalidArgument, "a project has at most %d groups", MaxProjectGroups)
	}
	out := make([]string, 0, len(list))
	for _, g := range list {
		n, err := ParseGroupName(g)
		if err != nil {
			return nil, err
		}
		if slices.Contains(out, n) {
			return nil, Errf(CodeInvalidArgument, "group %q is listed twice", n)
		}
		out = append(out, n)
	}
	return out, nil
}

// CheckMemberGroup checks that g ("" = ungrouped) is one of groups, and
// returns it trimmed.
func CheckMemberGroup(groups []string, g string) (string, error) {
	g = strings.TrimSpace(g)
	if g != "" && !slices.Contains(groups, g) {
		return "", Errf(CodeInvalidArgument, "the project has no group %q", g).
			WithSuggestion("create the group first (PATCH the project's groups), or pick one of its groups")
	}
	return g, nil
}

// ApplyGroups replaces the project's groups (when groups is non-nil) and
// moves members into groups (assign: agent → group, "" = ungrouped).
// Members whose group is no longer listed become ungrouped (deleting a
// group); renaming one is a new list plus moving its members. It checks
// everything before changing anything, and returns the members whose
// group changed.
func (p *Project) ApplyGroups(members []*ProjectMember, groups *[]string, assign map[AgentName]string, now time.Time) ([]*ProjectMember, error) {
	next := p.Groups
	if groups != nil {
		var err error
		if next, err = ParseProjectGroups(*groups); err != nil {
			return nil, err
		}
	}
	byName := make(map[AgentName]*ProjectMember, len(members))
	for _, m := range members {
		byName[m.Agent] = m
	}
	want := make(map[AgentName]string, len(assign))
	for a, g := range assign {
		if byName[a] == nil {
			return nil, Errf(CodeInvalidArgument, "%s is not a member of project %s", a, p.Name).
				WithSuggestion("add it to the project first")
		}
		g, err := CheckMemberGroup(next, g)
		if err != nil {
			return nil, err
		}
		want[a] = g
	}
	var changed []*ProjectMember
	for _, m := range members {
		g, ok := want[m.Agent]
		if !ok {
			g = m.Group
			if g != "" && !slices.Contains(next, g) {
				g = ""
			}
		}
		if g != m.Group {
			m.Group = g
			changed = append(changed, m)
		}
	}
	p.Groups = append([]string{}, next...)
	p.UpdatedAt = now
	return changed, nil
}

// CheckDocBody validates a document's body.
func CheckDocBody(s string) error {
	if len(s) > maxProjectDoc {
		return Errf(CodeInvalidArgument, "the document is longer than %d bytes", maxProjectDoc).
			WithSuggestion("split it, or keep it in the repository and add a link")
	}
	return nil
}

// ProjectMemoryDoc names the project document (memory.md once pulled) that holds the project's
// memory: what its members learned and should all keep in mind (decisions,
// conventions, the owner's preferences). Any member may add an entry; its
// editors change or tidy it like any document.
const ProjectMemoryDoc = "memory"

// maxMemoryEntry bounds one memory entry.
const maxMemoryEntry = 1000

// AppendMemory adds an entry to a memory document's body: one list item,
// with who wrote it and when.
func AppendMemory(body, text, by string, now time.Time) (string, error) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "", Errf(CodeInvalidArgument, "the memory entry is empty")
	}
	if utf8.RuneCountInString(text) > maxMemoryEntry {
		return "", Errf(CodeInvalidArgument, "a memory entry is at most %d characters", maxMemoryEntry).
			WithSuggestion("say it in a sentence or two; put details in a project document and mention its name")
	}
	if strings.TrimSpace(body) == "" {
		body = "# Project memory\n"
	}
	entry := "- " + text
	if by != "" {
		entry += " (" + by + ", " + now.Format("2006-01-02") + ")"
	}
	out := strings.TrimRight(body, "\n") + "\n" + entry + "\n"
	if len(out) > maxProjectDoc {
		return "", Errf(CodeInvalidArgument, "the project memory is full (%d bytes)", maxProjectDoc).
			WithSuggestion("ask a coordinator or the project's owner to tidy the memory document")
	}
	return out, nil
}

// ProjectRepository stores projects, their members and documents.
type ProjectRepository interface {
	Create(ctx context.Context, p *Project) error
	Save(ctx context.Context, p *Project) error
	// SaveStorage updates quota overrides without replacing ordinary project fields.
	SaveStorage(ctx context.Context, p *Project) error
	Get(ctx context.Context, name string) (*Project, error)
	List(ctx context.Context) ([]*Project, error)
	Delete(ctx context.Context, id string) error

	PutMember(ctx context.Context, m *ProjectMember) error
	// SaveGroups stores the project's groups and the groups of the given
	// members in one transaction.
	SaveGroups(ctx context.Context, p *Project, members []*ProjectMember) error
	DeleteMember(ctx context.Context, project string, agent AgentName) error
	Members(ctx context.Context, project string) ([]*ProjectMember, error)
	// MembershipsOf lists the projects (ids) an agent is in.
	MembershipsOf(ctx context.Context, agent AgentName) ([]*ProjectMember, error)
	// DeleteAgent removes an agent from every project (it was removed).
	DeleteAgent(ctx context.Context, agent AgentName) error

	PutDoc(ctx context.Context, d *ProjectDoc) error
	GetDoc(ctx context.Context, project, name string) (*ProjectDoc, error)
	DeleteDoc(ctx context.Context, project, name string) error
	Docs(ctx context.Context, project string) ([]*ProjectDoc, error)
}
