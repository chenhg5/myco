package console

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
)

// The computer that runs the local hub is a machine like any other: it joins
// its own hub, so the hub has a machine to start sessions on and its agents
// can be reached there. Before v0.18.11 `myco hub up` from the app left this
// out; the app adds it once, in the background, for hubs set up earlier.

// localHubJoined reports whether machine.yaml has a membership of the local
// hub (by its local address or the address its links carry).
func (l *Local) localHubJoined(ctx context.Context, localURL string) bool {
	m, err := config.LoadMachine(filepath.Join(l.Home, ".myco", "config.yaml"))
	if err != nil || m == nil {
		return false
	}
	if _, ok := m.For(localURL); ok {
		return true
	}
	c, err := client.New(localURL, "")
	if err != nil {
		return false
	}
	hctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if h, err := c.Health(hctx); err == nil && h.PublicURL != "" {
		_, ok := m.For(h.PublicURL)
		return ok
	}
	return false
}

// joinLocalHub makes this computer a machine of its local hub: a one-use
// join link from the hub's admin token, then `myco machine join`. A no-op
// when it already is one.
func (l *Local) joinLocalHub(ctx context.Context) error {
	env := l.hubEnv()
	localURL, admin := env[config.EnvHub], env[config.EnvAdminToken]
	if localURL == "" || admin == "" {
		return errors.New("~/.myco/hub/hub.env has no hub address or admin token")
	}
	if l.localHubJoined(ctx, localURL) {
		return nil
	}
	out, err := l.runIn(ctx, "", []string{config.EnvHub + "=" + localURL, config.EnvAdminToken + "=" + admin},
		"link", "create", "--uses", "1", "--expires", "15m", "--note", "this computer (myco app)")
	if err != nil {
		return err
	}
	var link struct {
		JoinURL string `json:"join_url"`
	}
	if json.Unmarshal(out, &link) != nil || link.JoinURL == "" {
		return errors.New("myco link create did not return a join link")
	}
	args := []string{"machine", "join", "--link", link.JoinURL}
	// A running runner picks the new machine node up by itself (it rescans
	// its profiles); installing the service again would restart it in the
	// middle of whatever its agents are doing. Without a service manager the
	// app runs the runner itself.
	if l.NoServiceManager || l.runnerRunning(ctx) {
		args = append(args, "--no-service")
	}
	_, err = l.run(ctx, args...)
	return err
}

// runnerRunning reports whether this computer's runner service is running.
func (l *Local) runnerRunning(ctx context.Context) bool {
	out, err := l.run(ctx, "runner", "service", "status")
	var st struct {
		Running bool `json:"running"`
	}
	return err == nil && json.Unmarshal(out, &st) == nil && st.Running
}

// joinLocalHubOnce runs joinLocalHub in the background, once per app run.
func (l *Local) joinLocalHubOnce() {
	l.joinOnce.Do(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			if err := l.joinLocalHub(ctx); err != nil {
				slog.Default().With("component", "local-hub").Warn("could not add this computer to the local hub", "err", err)
			}
		}()
	})
}
