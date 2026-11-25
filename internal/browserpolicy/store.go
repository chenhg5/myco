// Package browserpolicy stores owner-reviewed browser script snapshots locally.
// Shared page records and browser component responses never grant permission.
package browserpolicy

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/myco-ai/myco/internal/api"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxSnapshot = 1 << 20
const maxPolicy = 16 << 20

var ErrDenied = errors.New("browser script is not approved for this profile and policy binding")
var ErrConflict = errors.New("this script version already has a different approved snapshot; save a new version or revoke it first")

// Scope must come from the native account/workspace identity, never page args.
// Page namespaces script ids without separating a profile's policy generation.
type Scope struct{ Workspace, Owner, Page string }
type Request = api.BrowserApprovalRequest
type Approval = api.BrowserApproval
type Snapshot struct {
	Approval Approval                   `json:"approval"`
	Page     string                     `json:"page"`
	Script   json.RawMessage            `json:"script"`
	Scripts  map[string]json.RawMessage `json:"scripts,omitempty"`
}
type Policy struct {
	Profile   string     `json:"profile"`
	Kind      string     `json:"kind,omitempty"`
	Name      string     `json:"name"`
	Version   int        `json:"version"`
	Enabled   bool       `json:"enabled"`
	Binding   string     `json:"binding"`
	Snapshots []Snapshot `json:"snapshots"`
}

// Store serializes edits within and across desktop processes. Reads observe
// complete atomic snapshots without creating files or blocking each other.
// A busy file lock fails closed; policy edits never overwrite another App's
// newer generation from an in-memory cache.
type Store struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) *Store { return &Store{dir: dir} }
func (s *Store) path(scope Scope, profile string) (string, error) {
	if s.dir == "" || scope.Workspace == "" || scope.Owner == "" || profile == "" {
		return "", ErrDenied
	}
	sum := sha256.Sum256([]byte(profile))
	return filepath.Join(s.dir, namespace(scope), hex.EncodeToString(sum[:])+".json"), nil
}
func (s *Store) load(path string) (Policy, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Policy{Version: 1}, nil
	}
	if err != nil {
		return Policy{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPolicy+1))
	if err != nil {
		return Policy{}, err
	}
	if len(data) > maxPolicy {
		return Policy{}, fmt.Errorf("browser policy is too large")
	}
	var p Policy
	if err = json.Unmarshal(data, &p); err != nil {
		return Policy{}, err
	}
	if p.Version != 1 {
		return Policy{}, fmt.Errorf("unsupported browser policy version")
	}
	return p, nil
}
func (s *Store) save(path string, p Policy) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > maxPolicy {
		return fmt.Errorf("browser policy is too large")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".policy-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// SetEnabled changes a profile's policy before the host may advertise it.
// Every expansion or revocation gets a fresh generation; old grants never
// silently gain newly approved scripts. Callers cancel native runs first.
func (s *Store) SetEnabled(scope Scope, profile string, enabled bool, name ...string) (Policy, error) {
	return s.setEnabled(scope, profile, enabled, "", name...)
}

// EnsureAgentProfile coordinates first-time provisioning across desktop processes.
func (s *Store) EnsureAgentProfile(scope Scope, create func() (string, string, error), activate func(string) error) (Policy, error) {
	path, err := s.path(scope, "@dedicated-agent-profile")
	if err != nil {
		return Policy{}, err
	}
	unlock, err := s.lock(path)
	if err != nil {
		return Policy{}, err
	}
	defer unlock()
	profiles, err := s.Profiles(scope)
	if err != nil {
		return Policy{}, err
	}
	var pending Policy
	for _, p := range profiles {
		switch p.Kind {
		case "agent":
			return p, nil
		case "agent_pending":
			pending = p
		}
	}
	if pending.Profile == "" {
		id, name, err := create()
		if err != nil {
			return Policy{}, err
		}
		pending, err = s.setEnabled(scope, id, false, "agent_pending", name)
		if err != nil {
			return Policy{}, err
		}
	}
	if err := activate(pending.Profile); err != nil {
		return Policy{}, err
	}
	return s.setEnabled(scope, pending.Profile, true, "agent")
}

func (s *Store) setEnabled(scope Scope, profile string, enabled bool, kind string, name ...string) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(scope, profile)
	if err != nil {
		return Policy{}, err
	}
	unlock, err := s.lock(path)
	if err != nil {
		return Policy{}, err
	}
	defer unlock()
	p, err := s.loadScoped(path, scope, profile)
	if err != nil {
		return Policy{}, err
	}
	profileName := p.Name
	if len(name) > 0 && name[0] != "" {
		profileName = name[0]
	}
	if kind == "" {
		kind = p.Kind
	}
	if p.Enabled == enabled && p.Name == profileName && p.Kind == kind {
		return p, nil
	}
	p.Name, p.Kind = profileName, kind
	p.Enabled, p.Binding = enabled, "bp_"+rand.Text()
	if err = s.save(path, p); err != nil {
		return Policy{}, err
	}
	return p, nil
}
func canonical(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > maxSnapshot {
		return nil, fmt.Errorf("invalid browser snapshot size")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v map[string]any
	if err := d.Decode(&v); err != nil || v == nil {
		return nil, fmt.Errorf("browser script must be a JSON object")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing browser script data")
	}
	return json.Marshal(v)
}
func snapshot(req Request, page string, now time.Time) (Snapshot, error) {
	if page == "" || req.ScriptID == "" || len(req.ScriptID) > 200 || req.Version < 1 || req.Profile == "" || len(req.Scripts) > 32 {
		return Snapshot{}, ErrDenied
	}
	script, err := canonical(req.Script)
	if err != nil {
		return Snapshot{}, err
	}
	var header struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(script, &header)
	if header.Name == "" || len(header.Name) > 200 {
		return Snapshot{}, fmt.Errorf("browser script needs a name")
	}
	children := map[string]json.RawMessage{}
	for id, raw := range req.Scripts {
		if id == "" || len(id) > 200 {
			return Snapshot{}, ErrDenied
		}
		child, err := canonical(raw)
		if err != nil {
			return Snapshot{}, err
		}
		children[id] = child
	}
	snap := Snapshot{Approval: Approval{ScriptID: req.ScriptID, Version: req.Version, Profile: req.Profile, Name: header.Name, ApprovedAt: now.UTC()}, Page: page, Script: script, Scripts: children}
	digest, err := snapshotDigest(snap)
	if err != nil {
		return Snapshot{}, err
	}
	snap.Approval.Digest = digest
	return snap, nil
}
func snapshotDigest(snap Snapshot) (string, error) {
	if snap.Scripts == nil {
		snap.Scripts = map[string]json.RawMessage{}
	}
	data, err := json.Marshal(struct {
		Page, Profile, ID string
		Version           int64
		Script            json.RawMessage
		Scripts           map[string]json.RawMessage
	}{snap.Page, snap.Approval.Profile, snap.Approval.ScriptID, snap.Approval.Version, snap.Script, snap.Scripts})
	if err != nil {
		return "", err
	}
	if len(data) > maxSnapshot {
		return "", fmt.Errorf("browser snapshot and child scripts are too large")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// ApproveConfirmed is called ONLY after the native trusted UI has reviewed
// this exact snapshot, including child scripts. It must not be bound directly
// to a page request. No shared records are read here or during execution.
func (s *Store) ApproveConfirmed(scope Scope, req Request, now time.Time) (Approval, string, error) {
	if _, err := Prepare(scope, req); err != nil {
		return Approval{}, "", err
	}
	snap, err := snapshot(req, scope.Page, now)
	if err != nil {
		return Approval{}, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(scope, req.Profile)
	if err != nil {
		return Approval{}, "", err
	}
	unlock, err := s.lock(path)
	if err != nil {
		return Approval{}, "", err
	}
	defer unlock()
	p, err := s.loadScoped(path, scope, req.Profile)
	if err != nil {
		return Approval{}, "", err
	}
	for _, old := range p.Snapshots {
		if old.Page == scope.Page && old.Approval.ScriptID == req.ScriptID && old.Approval.Version == req.Version {
			if old.Approval.Digest != snap.Approval.Digest {
				return Approval{}, "", ErrConflict
			}
			return old.Approval, p.Binding, nil
		}
	}
	if len(p.Snapshots) >= 128 {
		return Approval{}, "", fmt.Errorf("too many approved browser versions; revoke unused versions")
	}
	p.Snapshots = append(p.Snapshots, snap)
	p.Binding = "bp_" + rand.Text()
	if err = s.save(path, p); err != nil {
		return Approval{}, "", err
	}
	return snap.Approval, p.Binding, nil
}
func (s *Store) Policy(scope Scope, profile string) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(scope, profile)
	if err != nil {
		return Policy{}, err
	}
	return s.loadScoped(path, scope, profile)
}

// Approved verifies the live generation and returns a private immutable copy.
func (s *Store) Approved(scope Scope, profile, binding, id string, version int64) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(scope, profile)
	if err != nil {
		return Snapshot{}, err
	}
	p, err := s.loadScoped(path, scope, profile)
	if err != nil {
		return Snapshot{}, err
	}
	if !p.Enabled || binding == "" || p.Binding != binding {
		return Snapshot{}, ErrDenied
	}
	var found *Snapshot
	for _, snap := range p.Snapshots {
		if (scope.Page == "" || snap.Page == scope.Page) && snap.Approval.ScriptID == id && snap.Approval.Version == version && snap.Approval.Profile == profile {
			digest, err := snapshotDigest(snap)
			if err != nil || digest != snap.Approval.Digest {
				return Snapshot{}, ErrDenied
			}
			if found != nil {
				return Snapshot{}, errors.New("ambiguous browser script; specify page_id")
			}
			copy := snap
			found = &copy
		}
	}
	if found != nil {
		return *found, nil
	}
	return Snapshot{}, ErrDenied
}

// Revoke removes a version (zero means all versions of that script) only on
// this page. The caller must stop/refuse jobs before withdrawing the Hub host.
func (s *Store) Revoke(scope Scope, profile, id string, version int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(scope, profile)
	if err != nil {
		return "", err
	}
	unlock, err := s.lock(path)
	if err != nil {
		return "", err
	}
	defer unlock()
	p, err := s.loadScoped(path, scope, profile)
	if err != nil {
		return "", err
	}
	keep := make([]Snapshot, 0, len(p.Snapshots))
	changed := false
	for _, snap := range p.Snapshots {
		if snap.Page == scope.Page && snap.Approval.ScriptID == id && (version == 0 || snap.Approval.Version == version) {
			changed = true
		} else {
			keep = append(keep, snap)
		}
	}
	if !changed {
		return p.Binding, nil
	}
	p.Snapshots = keep
	p.Binding = "bp_" + rand.Text()
	if err = s.save(path, p); err != nil {
		return "", err
	}
	return p.Binding, nil
}

func (s *Store) lock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return lockPolicy(path + ".lock")
}

func namespace(scope Scope) string {
	raw, _ := json.Marshal([]string{scope.Workspace, scope.Owner})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *Store) loadScoped(path string, scope Scope, profile string) (Policy, error) {
	p, err := s.load(path)
	if err != nil {
		return Policy{}, err
	}
	if p.Profile == "" && p.Binding == "" && !p.Enabled && len(p.Snapshots) == 0 {
		p.Profile, p.Name = profile, profile
	}
	if p.Profile != profile {
		return Policy{}, ErrDenied
	}
	return p, nil
}

// Profiles enumerates native opt-ins for exactly this workspace/account. No
// component-wide legacy agents preference can become a scoped opt-in.
func (s *Store) Profiles(scope Scope) ([]Policy, error) {
	if scope.Workspace == "" || scope.Owner == "" || s.dir == "" {
		return nil, ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.dir, namespace(scope))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Policy{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Policy{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		p, err := s.load(path)
		if err != nil {
			return nil, err
		}
		expected, e := s.path(scope, p.Profile)
		if e != nil || expected != path {
			return nil, ErrDenied
		}
		out = append(out, p)
	}
	return out, nil
}

// HostLease prevents two App processes from replacing each other's host for
// the same workspace/account/profile. Policy writes use a different lock.
func (s *Store) HostLease(scope Scope, profile string) (func(), error) {
	path, err := s.path(scope, profile)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return lockPolicy(path + ".host.lock")
}
