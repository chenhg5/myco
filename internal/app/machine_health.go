package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/myco-ai/myco/internal/domain"
)

func (s *MachineService) SetHealthProjects(p domain.ProjectRepository) { s.healthProjects = p }

// Health notifications contain aggregate numbers only. One transition in the
// runner's persisted runtime produces one note per coordinator, even
// when that coordinator belongs to several affected projects.
func (s *MachineService) notifyHealth(ctx context.Context, a *domain.Agent, old, rt domain.Runtime) {
	if s.healthProjects == nil || rt.MachineHealth == "" {
		return
	}
	var h domain.MachineHealth
	if json.Unmarshal([]byte(rt.MachineHealth), &h) != nil || !h.Complete {
		return
	}
	_ = old
	if h.Incident == "" {
		return
	}
	machine, err := s.machines.Get(ctx, a.MachineID)
	if err != nil || machine.Owner != a.Owner {
		return
	}
	projects, err := s.healthProjects.List(ctx)
	if err != nil {
		return
	}
	agents, err := s.agents.List(ctx)
	if err != nil {
		return
	}
	here := map[domain.AgentName]bool{}
	for _, agent := range agents {
		if agent.MachineID == machine.ID {
			here[agent.Name] = true
		}
	}
	recipients := map[domain.AgentName]bool{}
	for _, p := range projects {
		if p.Archived {
			continue
		}
		members, err := s.healthProjects.Members(ctx, p.ID)
		if err != nil {
			continue
		}
		related := slices.Contains(p.Machines, machine.ID)
		for _, member := range members {
			related = related || here[member.Agent]
		}
		if !related {
			continue
		}
		for _, member := range members {
			if member.Role == domain.ProjectRoleCoordinator {
				recipients[member.Agent] = true
			}
		}
	}
	state := "recovered"
	if h.Alert {
		state = "needs attention"
	}
	body := fmt.Sprintf("Machine %s %s. %s: %.0f%%; available memory: %.0f%%. Open Computers for a fresh sample and verified process controls. No processes were stopped automatically.", machine.Name, state, h.LoadMetric, h.LoadRatio*100, 100*float64(h.MemoryAvailable)/float64(max(h.MemoryTotal, 1)))
	for recipient := range recipients {
		_, _ = s.messages.send(ctx, &domain.Agent{Name: AdminSender}, SendInput{To: string(recipient), Type: string(domain.TypeNote), Body: body, IdempotencyKey: fmt.Sprintf("machine-health:%x", sha256.Sum256([]byte(machine.ID+"/"+h.Incident+"/"+state+"/"+string(recipient))))}, true)
	}
}
