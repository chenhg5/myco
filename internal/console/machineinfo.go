package console

import (
	"context"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness/codex"
)

// machineSessionsMax bounds the sessions a machine reports at a time.
const machineSessionsMax = 200

// MachineInventory is what a machine node tells its owner about its
// computer (api.MachineInfo): the agent CLIs, as Settings › 本机 agent shows
// them on this computer, and the recent Claude Code and Codex sessions, as
// 添加 agent lists them here, marked when one already joined the workspace
// at hub (or another one).
func MachineInventory(ctx context.Context, home, hub string) ([]api.MachineAI, []api.MachineSession) {
	l := &Local{Home: home, ShellPath: LoginShellPath}
	ais := []api.MachineAI{}
	for _, a := range l.LocalAgents(ctx) {
		ai := api.MachineAI{CLI: a.CLI, Name: a.Name, State: a.State, Version: a.Version, Install: a.Install, Login: a.Login}
		if a.CLI == "codex" && a.State != "missing" {
			for _, m := range codex.ModelsIn(l.codexHome()) {
				ai.Models = append(ai.Models, api.RuntimeModel{ID: m.ID, Name: m.Name, Efforts: m.Efforts})
			}
		}
		ais = append(ais, ai)
	}
	sessions := []api.MachineSession{}
	for _, s := range l.LocalSessions(time.Now()) {
		if len(sessions) >= machineSessionsMax {
			break
		}
		ms := api.MachineSession{CLI: s.CLI, SessionID: s.SessionID, Workdir: s.Workdir, LastActive: s.LastActive, Title: s.Title, Auto: s.Auto}
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

// inventoryTTL is how long a machine's inventory is answered from memory
// before it is read again (in the background: the answer never waits for
// a fresh one once there is one).
const inventoryTTL = 20 * time.Second

type inventory struct {
	ais      []api.MachineAI
	sessions []api.MachineSession
	at       time.Time
}

var (
	inventoryMu      sync.Mutex
	inventoryCache   = map[string]*inventory{}
	inventoryRunning = map[string]chan struct{}{}
)

// MachineInventoryCached answers from the last inventory read (refreshing
// it in the background when older than inventoryTTL), so asking a
// computer what it has comes back at once; the first ask after a start
// reads it. PrewarmMachineInventory reads it ahead of the first ask.
func MachineInventoryCached(ctx context.Context, home, hub string) ([]api.MachineAI, []api.MachineSession) {
	key := home + "\x00" + hub
	inventoryMu.Lock()
	inv := inventoryCache[key]
	inventoryMu.Unlock()
	if inv == nil {
		inv = refreshInventory(ctx, home, hub)
	} else if time.Since(inv.at) > inventoryTTL {
		go refreshInventory(context.WithoutCancel(ctx), home, hub)
	}
	return inv.ais, inv.sessions
}

// PrewarmMachineInventory reads the inventory in the background, unless a
// fresh one is there already.
func PrewarmMachineInventory(home, hub string) {
	inventoryMu.Lock()
	inv := inventoryCache[home+"\x00"+hub]
	inventoryMu.Unlock()
	if inv != nil && time.Since(inv.at) < inventoryTTL {
		return
	}
	go refreshInventory(context.Background(), home, hub)
}

// ForgetMachineInventory drops the inventory read (a session just joined
// or started: the next ask reads it again).
func ForgetMachineInventory(home, hub string) {
	inventoryMu.Lock()
	delete(inventoryCache, home+"\x00"+hub)
	inventoryMu.Unlock()
}

// refreshInventory reads it once at a time per key; a second caller waits
// for the read in progress.
func refreshInventory(ctx context.Context, home, hub string) *inventory {
	key := home + "\x00" + hub
	inventoryMu.Lock()
	if ch, ok := inventoryRunning[key]; ok {
		inventoryMu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
		}
		inventoryMu.Lock()
		inv := inventoryCache[key]
		inventoryMu.Unlock()
		if inv == nil {
			inv = &inventory{ais: []api.MachineAI{}, sessions: []api.MachineSession{}}
		}
		return inv
	}
	ch := make(chan struct{})
	inventoryRunning[key] = ch
	inventoryMu.Unlock()
	ais, sessions := MachineInventory(ctx, home, hub)
	inv := &inventory{ais: ais, sessions: sessions, at: time.Now()}
	inventoryMu.Lock()
	inventoryCache[key] = inv
	delete(inventoryRunning, key)
	inventoryMu.Unlock()
	close(ch)
	return inv
}
