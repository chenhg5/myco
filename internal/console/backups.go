package console

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/backup"
	"github.com/myco-ai/myco/internal/client"
)

// Backups of a workspace kept on this computer: a backup on the server's
// disk does not survive that disk. The App saves one on request, and with
// "keep a copy here" on, takes the newest from the workspace once a day
// (the hub makes one a day by itself), keeping the last localBackups.
// Only the workspace's owner gets them (the hub decides).

const localBackups = 3

// backupState is what the App remembers about backups (backups.json next
// to console-hubs.json): the workspaces whose backups it keeps here.
type backupState struct {
	Auto map[string]bool `json:"auto"` // hub id → keep a copy here
}

type localBackupsView struct {
	api.Backups
	// Local are the copies on this computer, in Dir, newest first.
	Local []backup.Info `json:"local"`
	Dir   string        `json:"dir"`
	Auto  bool          `json:"auto"`
}

var backupMu sync.Mutex // backups.json, and one download at a time

func (b *Backend) backupStatePath() string {
	if b.Store == nil || b.Store.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(b.Store.path), "backups.json")
}

func (b *Backend) loadBackupState() backupState {
	st := backupState{Auto: map[string]bool{}}
	if p := b.backupStatePath(); p != "" {
		if raw, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(raw, &st)
		}
	}
	if st.Auto == nil {
		st.Auto = map[string]bool{}
	}
	return st
}

func (b *Backend) saveBackupState(st backupState) error {
	p := b.backupStatePath()
	if p == "" {
		return nil
	}
	raw, _ := json.MarshalIndent(st, "", "  ")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

// localBackupDir is where this computer keeps a workspace's backups:
// ~/myco-backups/<workspace name>.
func (b *Backend) localBackupDir(h Hub) string {
	home := ""
	if b.Local != nil {
		home = b.Local.Home
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	name := strings.TrimSpace(unsafeName.ReplaceAllString(h.Name, "-"))
	if name == "" || strings.Trim(name, ".") == "" {
		name = h.ID
	}
	return filepath.Join(home, "myco-backups", name)
}

func (b *Backend) backupRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/backups", b.withHubAndHubTimeout(30*time.Second, func(w http.ResponseWriter, r *http.Request, c *client.Client, h Hub) {
		list, err := c.Backups(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		dir := b.localBackupDir(h)
		local, _ := backup.List(dir)
		writeJSON(w, http.StatusOK, localBackupsView{Backups: *list, Local: local, Dir: dir, Auto: b.loadBackupState().Auto[h.ID]})
	}))
	m.HandleFunc("POST /app/hubs/{id}/backups", b.withHubTimeout(30*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		bk, err := c.CreateBackup(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, bk)
	}))
	// Save one here (the newest when no name is given).
	m.HandleFunc("POST /app/hubs/{id}/backups/save", b.withHubAndHubTimeout(60*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client, h Hub) {
		var in struct {
			Name string `json:"name"`
		}
		if r.ContentLength != 0 && !decode(w, r, &in) {
			return
		}
		path, err := b.saveBackup(r.Context(), c, h, in.Name)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"path": path, "dir": filepath.Dir(path)})
	}))
	m.HandleFunc("PUT /app/hubs/{id}/backups/auto", b.withHubAndHubTimeout(30*time.Second, func(w http.ResponseWriter, r *http.Request, c *client.Client, h Hub) {
		var in struct {
			On bool `json:"on"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.On {
			// Only its owner may: ask the hub before keeping the setting.
			if _, err := c.Backups(r.Context()); err != nil {
				writeErr(w, err)
				return
			}
		}
		backupMu.Lock()
		st := b.loadBackupState()
		if in.On {
			st.Auto[h.ID] = true
		} else {
			delete(st.Auto, h.ID)
		}
		err := b.saveBackupState(st)
		backupMu.Unlock()
		if err != nil {
			writeErr(w, err)
			return
		}
		if in.On {
			go b.keepBackups(context.Background())
		}
		writeJSON(w, http.StatusOK, map[string]bool{"auto": in.On})
	}))
	m.HandleFunc("POST /app/hubs/{id}/backups/reveal", b.withHubAndHubTimeout(10*time.Second, func(w http.ResponseWriter, r *http.Request, c *client.Client, h Hub) {
		dir := b.localBackupDir(h)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			writeErr(w, err)
			return
		}
		// The newest copy, selected in its folder (or the folder itself).
		target := dir
		if local, _ := backup.List(dir); len(local) > 0 {
			target = filepath.Join(dir, local[0].Name)
		}
		revealFile(target)
		writeJSON(w, http.StatusOK, map[string]string{"dir": dir})
	}))
	b.backupLoop.Do(func() { go b.backupTicker() })
}

// saveBackup downloads backup name (the newest when "") of hub h into its
// folder here and keeps the newest localBackups there.
func (b *Backend) saveBackup(ctx context.Context, c *client.Client, h Hub, name string) (string, error) {
	if name == "" {
		list, err := c.Backups(ctx)
		if err != nil {
			return "", err
		}
		if len(list.Backups) == 0 {
			bk, err := c.CreateBackup(ctx)
			if err != nil {
				return "", err
			}
			name = bk.Name
		} else {
			name = list.Backups[0].Name
		}
	}
	dir := b.localBackupDir(h)
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return filepath.Join(dir, name), nil // already here
	}
	backupMu.Lock()
	defer backupMu.Unlock()
	path, err := c.DownloadBackup(ctx, name, dir)
	if err != nil {
		return "", err
	}
	_ = backup.Prune(dir, localBackups)
	return path, nil
}

// backupTicker keeps the copies here up to date, once an hour.
func (b *Backend) backupTicker() {
	time.Sleep(2 * time.Minute) // after the App has started up
	for {
		b.keepBackups(context.Background())
		time.Sleep(time.Hour)
	}
}

// keepBackups takes the newest backup of each workspace with "keep a copy
// here" on, when it is newer than the newest copy here.
func (b *Backend) keepBackups(ctx context.Context) {
	st := b.loadBackupState()
	if len(st.Auto) == 0 || b.Store == nil {
		return
	}
	hubs, err := b.Store.List()
	if err != nil {
		return
	}
	for _, h := range hubs {
		if !st.Auto[h.ID] {
			continue
		}
		c, err := b.client(h)
		if err != nil {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 60*time.Minute)
		list, err := c.Backups(cctx)
		if err == nil && len(list.Backups) > 0 {
			local, _ := backup.List(b.localBackupDir(h))
			newest := list.Backups[0]
			sort.Slice(local, func(i, j int) bool { return local[i].CreatedAt.After(local[j].CreatedAt) })
			have := false
			for _, l := range local {
				have = have || l.Name == newest.Name
			}
			if !have {
				if _, err := b.saveBackup(cctx, c, h, newest.Name); err != nil {
					slog.Warn("keeping a backup here failed", "hub", h.Name, "err", err)
				}
			}
		}
		cancel()
	}
}
