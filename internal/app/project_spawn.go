package app

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/myco-ai/myco/internal/domain"
)

// projectSpawnGrant reports whether a project lets sender start a session
// on recipient's computer, and which ("project:<name>"; "" when none).
//
// Which computers a project may start agents on is a setting of its own
// (Project.Machines, chosen by its owner; not the terminals it has). The
// project's coordinators may ask those computers' machine nodes for new
// agent sessions without being on the nodes' allow lists. Not for new
// terminals (a shell node runs whatever it is sent), not with a sealed body
// (the hub cannot tell what it asks), not for archived projects. The
// computer's own rules (spawn.dirs, access) still apply when it starts the
// session.
func (s *MessageService) projectSpawnGrant(ctx context.Context, sender, recipient *domain.Agent, typ domain.MessageType, body string) string {
	if s.projects == nil || typ != domain.TypeTask || recipient.Kind() != domain.KindMachine || recipient.MachineID == "" {
		return ""
	}
	var req struct {
		CLI       string   `json:"cli"`
		JoinLink  string   `json:"join_link"`
		AllowFrom []string `json:"allow_from"`
	}
	if domain.IsSealed(body) || json.Unmarshal([]byte(body), &req) != nil {
		return ""
	}
	if req.CLI == "" || req.CLI == "exec" || req.JoinLink != "" || len(req.AllowFrom) > 0 {
		return ""
	}
	mine, err := s.projects.MembershipsOf(ctx, sender.Name)
	if err != nil {
		return ""
	}
	var projects []*domain.Project
	for _, pm := range mine {
		if pm.Role != domain.ProjectRoleCoordinator {
			continue
		}
		if projects == nil {
			if projects, err = s.projects.List(ctx); err != nil {
				return ""
			}
		}
		for _, p := range projects {
			if p.ID == pm.Project && !p.Archived && slices.Contains(p.Machines, recipient.MachineID) {
				return "project:" + p.Name
			}
		}
	}
	return ""
}
