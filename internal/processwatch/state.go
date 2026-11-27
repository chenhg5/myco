package processwatch

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

type savedScope struct {
	Owner   Owner
	Root    Process
	Members []Process
}
type savedRegistry struct {
	Runner Process
	Scopes []savedScope
}

// NewPersistent recovers identity-verified Unix descendants left by a crashed
// runner. Files contain ownership metadata only, never argv or environment.
// A live registry is never adopted, and a restarted sample begins a new timer.
func NewPersistent(instance, dir string) *Manager {
	m := New(instance)
	if runtime.GOOS != "windows" {
		m.stateDir = dir
		m.restore()
	}
	return m // Windows Job handles kill their children on runner exit.
}
func (m *Manager) restore() {
	entries, err := os.ReadDir(m.stateDir)
	if err != nil {
		return
	}
	// Inspect recent registries first, with a bounded startup I/O budget.
	sort.Slice(entries, func(i, j int) bool {
		a, ae := entries[i].Info()
		b, be := entries[j].Info()
		return ae == nil && be == nil && a.ModTime().After(b.ModTime())
	})
	var adopted []string
	for i, entry := range entries {
		if i >= 64 {
			break
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(m.stateDir, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
		file.Close()
		if err != nil || len(data) > 4<<20 {
			continue
		}
		var saved savedRegistry
		if json.Unmarshal(data, &saved) != nil {
			continue
		}
		if p, e := m.lookup(saved.Runner.PID); e == nil && p.Birth == saved.Runner.Birth {
			continue
		}
		count := len(m.scopes)
		for _, record := range saved.Scopes {
			if record.Root.PID <= 1 || record.Root.Birth == "" || record.Owner.Agent == "" || record.Owner.Workspace == "" {
				continue
			}
			var live []Process
			for _, p := range append(record.Members, record.Root) {
				if cur, e := m.lookup(p.PID); e == nil && cur.Birth == p.Birth {
					live = append(live, cur)
				}
			}
			if len(live) == 0 {
				continue
			}
			scope := &Scope{manager: m, owner: record.Owner, root: record.Root, members: map[int]Process{}}
			m.scopes = append(m.scopes, scope)
			for _, p := range live {
				scope.members[p.PID] = p
				m.previous[p.PID] = &observed{process: p, scope: scope}
			}
		}
		if len(m.scopes) == count {
			_ = os.Remove(path)
		} else {
			adopted = append(adopted, path)
		}
	}
	// Do not discard old ownership evidence unless the new registry is durable.
	if len(adopted) > 0 && m.saveLocked() {
		for _, path := range adopted {
			_ = os.Remove(path)
		}
	}
}
func (m *Manager) saveLocked() bool {
	if m.stateDir == "" {
		return false
	}
	runner, err := m.lookup(os.Getpid())
	if err != nil {
		return false
	}
	saved := savedRegistry{Runner: runner}
	for _, s := range m.scopes {
		if s.stopped {
			continue
		}
		record := savedScope{Owner: s.owner, Root: s.root}
		for _, p := range s.members {
			record.Members = append(record.Members, p)
		}
		saved.Scopes = append(saved.Scopes, record)
	}
	dest := filepath.Join(m.stateDir, m.instance+".json")
	if len(saved.Scopes) == 0 {
		err = os.Remove(dest)
		return err == nil || errors.Is(err, os.ErrNotExist)
	}
	data, err := json.Marshal(saved)
	if err != nil || len(data) > 4<<20 {
		return false
	}
	if os.MkdirAll(m.stateDir, 0700) != nil {
		return false
	}
	tmp, err := os.CreateTemp(m.stateDir, ".scopes-*")
	if err != nil {
		return false
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return false
	}
	if tmp.Sync() != nil {
		tmp.Close()
		return false
	}
	if tmp.Close() != nil {
		return false
	}
	return os.Rename(name, dest) == nil
}
