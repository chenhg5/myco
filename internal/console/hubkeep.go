package console

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/service"
)

// A workspace on this computer whose service is installed but not running
// (stopped by something other than the App's 停止, which removes it: the
// computer slept through a crash, launchd gave up, a script booted it out)
// leaves the App's own workspace unreachable, with "connection refused" on
// everything. The App starts it again, as it was set up (tunnel included),
// at most once a minute.

// serviceFor is the service manager of one of myco's services here (tests
// replace it).
var serviceFor = func(home string, id service.ID) (service.Manager, error) {
	return service.DetectID(home, service.ExecRunner{}, id)
}

type starter interface{ Start() error }

// startStoppedHub starts hub name's service when it is installed and not
// running; it reports whether it did.
func (l *Local) startStoppedHub(name string) bool {
	return l.startStopped(service.HubIDFor(name))
}

// startStopped starts service id when it is installed and not running (the
// runner too: uninstalling it removes it), at most once a minute; it
// reports whether it did.
func (l *Local) startStopped(id service.ID) bool {
	if l.NoServiceManager {
		return false
	}
	now := time.Now()
	if last, ok := l.hubStarts.Load(id.Label); ok && now.Sub(last.(time.Time)) < time.Minute {
		return false
	}
	l.hubStarts.Store(id.Label, now)
	m, err := serviceFor(l.Home, id)
	if err != nil {
		return false
	}
	st, err := m.Status()
	if err != nil || !st.Installed || st.Running {
		return false
	}
	if s, ok := m.(starter); ok {
		err = s.Start()
	} else {
		err = m.Restart()
	}
	if err != nil {
		slog.Default().Warn("could not start a stopped myco service", "service", id.Label, "err", err)
		return false
	}
	slog.Default().Info("started a stopped myco service", "service", id.Label)
	return true
}

// refused reports whether err is a hub that is not listening at all.
func refused(err error) bool {
	return err != nil && strings.Contains(err.Error(), "connection refused")
}

// revivedHub starts the local hub at url when it is stopped and waits for
// it to answer; it reports whether it now does.
func (l *Local) revivedHub(ctx context.Context, url string) bool {
	url = strings.TrimRight(url, "/")
	for _, name := range l.hubNames() {
		if strings.TrimRight(l.hubEnvFor(name)[config.EnvHub], "/") != url {
			continue
		}
		if !l.startStoppedHub(name) {
			return false
		}
		c, err := client.New(url, "")
		if err != nil {
			return false
		}
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			hctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, err := c.Health(hctx)
			cancel()
			if err == nil {
				return true
			}
			select {
			case <-ctx.Done():
				return false
			case <-time.After(500 * time.Millisecond):
			}
		}
		return false
	}
	return false
}
