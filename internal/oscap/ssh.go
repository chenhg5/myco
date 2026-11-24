package oscap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/myco-ai/myco/internal/sshcap"
)

var sshInitMu sync.Mutex

// SSHManager is the same private manager used by human calls and trusted
// agent-action hosts; shallow page-scoped Env copies share it.
func (e *Env) SSHManager() (*sshcap.Manager, error) {
	sshInitMu.Lock()
	defer sshInitMu.Unlock()
	if e.SSH == nil {
		e.SSH = &sshcap.Manager{Home: e.Home}
	}
	if e.SSH.Binary == "" {
		binary, err := e.LookPath("ssh")
		if err != nil {
			return nil, unsupported("OpenSSH")
		}
		e.SSH.Binary = binary
	}
	return e.SSH, nil
}
func sshError(err error) error {
	var e *sshcap.Error
	if errors.As(err, &e) {
		return &Error{Code: e.Code, Message: e.Message}
	}
	return AsError(err)
}
func init() {
	register(
		&Cap{Name: "ssh.profiles", About: "Manage private local OpenSSH profiles (never keys or passwords)", Missing: sshMissing, Fn: sshProfiles},
		&Cap{Name: "ssh.run", About: "Run bounded fixed Linux administration actions over strict OpenSSH", Missing: sshMissing, Fn: sshRun},
	)
}
func sshMissing(e *Env) string {
	if _, err := e.LookPath("ssh"); err != nil {
		return "OpenSSH is not installed"
	}
	return ""
}
func sshProfiles(ctx context.Context, e *Env, raw json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Op           string `json:"op"`
		ID           string `json:"id"`
		Name         string `json:"name"`
		Destination  string `json:"destination"`
		User         string `json:"user"`
		Port         int    `json:"port"`
		AgentEnabled bool   `json:"agent_enabled"`
	}
	if sshDecode(raw, &in) != nil {
		return nil, errf(CodeInvalid, "invalid SSH profile request")
	}
	m, err := e.SSHManager()
	if err != nil {
		return nil, err
	}
	var v any
	switch in.Op {
	case "list":
		p, er := m.List()
		v = map[string]any{"profiles": p}
		err = er
	case "create":
		v, err = m.Create(ctx, sshcap.Create{Name: in.Name, Destination: in.Destination, User: in.User, Port: in.Port})
	case "delete":
		err = m.Delete(in.ID)
		v = map[string]bool{"ok": err == nil}
	case "agent_enabled":
		hub, _, _ := strings.Cut(e.Scope, "\x00")
		if in.AgentEnabled && hub == "" {
			return nil, errf(CodeInvalid, "enable agent access from a workspace page")
		}
		v, err = m.SetAgentFor(ctx, in.ID, in.AgentEnabled, hub)
	default:
		return nil, errf(CodeInvalid, "profile op must be list, create, delete or agent_enabled")
	}
	if err != nil {
		return nil, sshError(err)
	}
	return v, nil
}
func sshRun(ctx context.Context, e *Env, raw json.RawMessage, _ Emit) (any, error) {
	var in struct {
		ProfileID string `json:"profile_id"`
		Binding   string `json:"binding"`
		sshcap.Operation
	}
	if sshDecode(raw, &in) != nil {
		return nil, errf(CodeInvalid, "invalid SSH operation")
	}
	m, err := e.SSHManager()
	if err != nil {
		return nil, err
	}
	v, err := m.Run(ctx, in.ProfileID, in.Binding, in.Operation, false)
	if err != nil {
		return nil, sshError(err)
	}
	return v, nil
}

func sshDecode(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}
