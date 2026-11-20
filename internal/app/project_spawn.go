package app

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

// projectSpawnGrant reports whether a project lets sender start a session
// on recipient's computer, and which ("project:<name>"; "" when none).
//
// A terminal (shell node) in a project says the project works on that
// computer, in that folder. So the project's agents may ask the computer's
// machine node for a new agent session there, in that folder or below it,
// without being on the node's allow list. Not for new terminals (a shell
// node runs whatever it is sent) and not with a sealed body (the hub cannot
// read the folder). The computer's own rules (spawn.dirs, access) still
// apply when it starts the session.
func (s *MessageService) projectSpawnGrant(ctx context.Context, sender, recipient *domain.Agent, typ domain.MessageType, body string) string {
	if s.projects == nil || typ != domain.TypeTask || recipient.Kind() != domain.KindMachine || recipient.MachineID == "" {
		return ""
	}
	var req struct {
		CLI       string   `json:"cli"`
		Dir       string   `json:"dir"`
		JoinLink  string   `json:"join_link"`
		AllowFrom []string `json:"allow_from"`
	}
	if domain.IsSealed(body) || json.Unmarshal([]byte(body), &req) != nil {
		return ""
	}
	if req.CLI == "" || req.CLI == "exec" || req.JoinLink != "" || len(req.AllowFrom) > 0 {
		return ""
	}
	dir := cleanDir(req.Dir)
	if dir == "" {
		return ""
	}
	mine, err := s.projects.MembershipsOf(ctx, sender.Name)
	if err != nil {
		return ""
	}
	projects, err := s.projects.List(ctx)
	if err != nil {
		return ""
	}
	for _, pm := range mine {
		var p *domain.Project
		for _, x := range projects {
			if x.ID == pm.Project {
				p = x
				break
			}
		}
		if p == nil || p.Archived {
			continue
		}
		members, err := s.projects.Members(ctx, p.ID)
		if err != nil {
			continue
		}
		for _, m := range members {
			if m.Agent == sender.Name {
				continue
			}
			a, err := s.agents.Get(ctx, m.Agent)
			if err != nil || a.Kind() != domain.KindShell || a.MachineID != recipient.MachineID {
				continue
			}
			if root := cleanDir(a.Binding.Workdir); root != "" && within(dir, root) {
				return "project:" + p.Name
			}
		}
	}
	return ""
}

// cleanDir puts a folder as the hub compares them: forward slashes, no
// trailing slash, no . or .. ("" for an empty or relative one).
func cleanDir(d string) string {
	d = strings.TrimSpace(strings.ReplaceAll(d, `\`, "/"))
	if d == "" {
		return ""
	}
	if !strings.HasPrefix(d, "/") && !strings.HasPrefix(d, "~") && !(len(d) > 2 && d[1] == ':') {
		return ""
	}
	return path.Clean(d)
}

// within reports whether dir is root or a folder inside it.
func within(dir, root string) bool {
	return dir == root || strings.HasPrefix(dir, strings.TrimSuffix(root, "/")+"/")
}
