package console

import (
	"context"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/config"
)

// machineSessionsMax bounds the sessions a machine reports at a time.
const machineSessionsMax = 40

// MachineInventory is what a machine node tells its owner about its
// computer (api.MachineInfo): the agent CLIs, as Settings › 本机 agent shows
// them on this computer, and the recent Claude Code and Codex sessions, as
// 添加 agent lists them here, marked when one already joined the workspace
// at hub (or another one).
func MachineInventory(ctx context.Context, home, hub string) ([]api.MachineAI, []api.MachineSession) {
	l := &Local{Home: home, ShellPath: LoginShellPath}
	ais := []api.MachineAI{}
	for _, a := range l.LocalAgents(ctx) {
		ais = append(ais, api.MachineAI{CLI: a.CLI, Name: a.Name, State: a.State, Version: a.Version, Install: a.Install, Login: a.Login})
	}
	sessions := []api.MachineSession{}
	for _, s := range l.LocalSessions(time.Now()) {
		if len(sessions) >= machineSessionsMax {
			break
		}
		ms := api.MachineSession{CLI: s.CLI, SessionID: s.SessionID, Workdir: s.Workdir, LastActive: s.LastActive, Title: s.Title}
		all := s.JoinedAll
		if len(all) == 0 && s.Joined != nil {
			all = []JoinedAs{*s.Joined}
		}
		for _, j := range all {
			if config.SameHub(j.Hub, hub) {
				ms.Joined = j.Agent
			} else {
				ms.JoinedElsewhere = true
			}
		}
		sessions = append(sessions, ms)
	}
	return ais, sessions
}
