package console

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
)

// Without a service manager (Windows), the app runs the local hub itself:
// `myco hub up --no-service` prepares it (admin token, tunnel checks) and
// prints the command, which the app starts in the background with no
// console window. The command is kept in ~/.myco/hub/app-hub.json, so the
// app starts the hub again when it starts; stopping removes it.

type appHub struct {
	Command []string `json:"command"`
	PID     int      `json:"pid,omitempty"`
}

func (l *Local) appHubFile() string { return filepath.Join(l.Home, ".myco", "hub", "app-hub.json") }

func (l *Local) appHubFileFor(name string) string {
	return filepath.Join(l.hubDir(name), "app-hub.json")
}

func (l *Local) readAppHub() *appHub {
	return l.readAppHubName("")
}

func (l *Local) readAppHubName(name string) *appHub {
	b, err := os.ReadFile(l.appHubFileFor(name))
	if err != nil {
		return nil
	}
	var h appHub
	if json.Unmarshal(b, &h) != nil || len(h.Command) == 0 {
		return nil
	}
	return &h
}

func (l *Local) writeAppHub(h *appHub) error {
	return l.writeAppHubName("", h)
}

func (l *Local) writeAppHubName(name string, h *appHub) error {
	b, _ := json.Marshal(h)
	return os.WriteFile(l.appHubFileFor(name), b, 0o600)
}

// startAppHub starts the hub command in the background (log in
// ~/.myco/hub/hub.log) and waits for it to answer.
func (l *Local) startAppHub(ctx context.Context, h *appHub) error {
	return l.startAppHubName(ctx, "", h)
}

func (l *Local) startAppHubName(ctx context.Context, name string, h *appHub) error {
	if l.StartProcess == nil {
		l.StartProcess = startDetached
	}
	pid, err := l.StartProcess(h.Command, filepath.Join(l.hubDir(name), "hub.log"), l.env())
	if err != nil {
		return fmt.Errorf("cannot start the hub: %w", err)
	}
	h.PID = pid
	if err := l.writeAppHubName(name, h); err != nil {
		return err
	}
	return l.waitHubUpName(ctx, name, 30*time.Second)
}

// waitHubUp waits until the local hub (hub.env's address) answers.
func (l *Local) waitHubUp(ctx context.Context, d time.Duration) error {
	return l.waitHubUpName(ctx, "", d)
}

func (l *Local) waitHubUpName(ctx context.Context, name string, d time.Duration) error {
	deadline := time.Now().Add(d)
	for {
		if u := l.hubEnvFor(name)[config.EnvHub]; u != "" {
			if c, err := client.New(u, ""); err == nil {
				cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				_, err := c.Health(cctx)
				cancel()
				if err == nil {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the hub did not start; see %s", filepath.Join(l.hubDir(name), "hub.log"))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// stopAppHub stops the hub the app started and forgets it.
func (l *Local) stopAppHub() error {
	return l.stopAppHubName("")
}

func (l *Local) stopAppHubName(name string) error {
	h := l.readAppHubName(name)
	if h == nil {
		return nil
	}
	// Only while the hub answers: a pid from before a reboot may belong to
	// another program by now.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	up := l.waitHubUpName(ctx, name, 0) == nil
	cancel()
	if h.PID > 0 && up {
		if p, err := os.FindProcess(h.PID); err == nil {
			_ = p.Kill()
		}
	}
	return os.Remove(l.appHubFileFor(name))
}

// keepAppHub starts the app-run hub again (once per app run) when it was
// running before and does not answer now.
func (l *Local) keepAppHub() {
	l.keepAppHubName("")
}

func (l *Local) keepAppHubName(name string) {
	if !l.NoServiceManager {
		return
	}
	l.appHubMu.Lock()
	if l.appHubStarted == nil {
		l.appHubStarted = map[string]bool{}
	}
	if l.appHubStarted[name] {
		l.appHubMu.Unlock()
		return
	}
	l.appHubStarted[name] = true
	l.appHubMu.Unlock()
	func() {
		h := l.readAppHubName(name)
		if h == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		up := l.waitHubUpName(ctx, name, 0) == nil
		cancel()
		if up {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			_ = l.startAppHubName(ctx, name, h)
		}()
	}()
}

// startDetached starts command in the background with output to logPath and
// returns its pid; it keeps running if the app quits.
func startDetached(command []string, logPath string, env []string) (int, error) {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout, cmd.Stderr, cmd.Env = f, f, env
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	return pid, nil
}
