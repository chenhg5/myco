package console

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserpolicy"
	"github.com/myco-ai/myco/internal/client"
)

type browserOwnerIdentity struct {
	URL, Owner string
	Credential [32]byte
}

func (b *Backend) browserPolicies() (*browserpolicy.Store, error) {
	b.browser.mu.Lock()
	defer b.browser.mu.Unlock()
	if b.browser.policies == nil {
		if b.Store == nil || !filepath.IsAbs(b.Store.path) {
			return nil, browserFailure("native_policy_unavailable", 409)
		}
		b.browser.policies = browserpolicy.New(filepath.Join(filepath.Dir(b.Store.path), "browser-agent-policies"))
	}
	return b.browser.policies, nil
}
func (b *Backend) browserCredential(h Hub) ([32]byte, bool) {
	if b.Secrets == nil {
		return [32]byte{}, false
	}
	token, err := b.Secrets.Get(h.ID)
	if err != nil || token == "" {
		return [32]byte{}, false
	}
	return sha256.Sum256([]byte(token)), true
}
func (b *Backend) cachedBrowserOwner(h Hub, page string) (browserpolicy.Scope, bool) {
	credential, ok := b.browserCredential(h)
	if !ok {
		return browserpolicy.Scope{}, false
	}
	b.browser.mu.Lock()
	defer b.browser.mu.Unlock()
	identity, ok := b.browser.identities[h.ID]
	if !ok || identity.URL != h.URL || identity.Credential != credential {
		return browserpolicy.Scope{}, false
	}
	return browserpolicy.Scope{Workspace: h.URL, Owner: identity.Owner, Page: page}, true
}
func (b *Backend) browserOwner(ctx context.Context, c *client.Client, h Hub, page string) (browserpolicy.Scope, error) {
	who, err := c.Whoami(ctx)
	if err != nil {
		return browserpolicy.Scope{}, err
	}
	if who.Device == nil || who.Member == nil || who.Member.ID == "" {
		return browserpolicy.Scope{}, browserFailure("permission_denied", 403)
	}
	if credential, ok := b.browserCredential(h); ok && credential == c.CredentialFingerprint() {
		b.browser.mu.Lock()
		if b.browser.identities == nil {
			b.browser.identities = map[string]browserOwnerIdentity{}
		}
		b.browser.identities[h.ID] = browserOwnerIdentity{URL: h.URL, Owner: who.Member.ID, Credential: credential}
		b.browser.mu.Unlock()
	}
	return browserpolicy.Scope{Workspace: h.URL, Owner: who.Member.ID, Page: page}, nil
}
func browserDecode(raw json.RawMessage, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return browserFailure("invalid_argument", 400)
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return browserFailure("invalid_argument", 400)
	}
	return nil
}
func browserPolicyError(err error) error {
	var graphError *browserpolicy.InvalidGraph
	if errors.As(err, &graphError) {
		return &client.Error{Code: "invalid_graph", Status: 400, Message: graphError.Error()}
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return &client.Error{Code: "native_policy_unavailable", Status: 409, Message: "The local browser policy could not be read or saved."}
	}
	if errors.Is(err, browserpolicy.ErrDenied) {
		return browserFailure("permission_denied", 403)
	}
	if errors.Is(err, browserpolicy.ErrConflict) {
		return &client.Error{Code: "conflict", Status: 409, Message: "This version has a different local approval; save a new version or revoke it first."}
	}
	return &client.Error{Code: "invalid_argument", Status: 400, Message: err.Error()}
}
func (b *Backend) blockBrowserAgents(hub, profile string) {
	b.browser.mu.Lock()
	if b.browser.blocked == nil {
		b.browser.blocked = map[string]bool{}
	}
	b.browser.blocked[hub+"/"+profile] = true
	stop := b.browser.cancelAgents
	b.browser.mu.Unlock()
	if stop != nil {
		stop(hub, profile)
	}
}
func (b *Backend) unblockBrowserAgents(hub, profile string) {
	b.browser.mu.Lock()
	delete(b.browser.blocked, hub+"/"+profile)
	b.browser.mu.Unlock()
}
func (b *Backend) browserAgentsBlocked(hub, profile string) bool {
	b.browser.mu.Lock()
	defer b.browser.mu.Unlock()
	return b.browser.blocked[hub+"/"+profile]
}
func (b *Backend) hasBrowserConsent(hub, page string) bool {
	g := b.grants()
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.perm(hub, page)
	return !p.denied["browser"] && (p.once["browser"] || p.always["browser"])
}

// Offline revocation only removes authority. It requires this page's existing
// native consent and an account identity cached under the current credential;
// it never fetches a Hub or silently switches to another account.
func (b *Backend) browserLocalRevocation(h Hub, in browserCall) (any, bool, error) {
	if in.Action != "profiles.allowAgents" && in.Action != "approvals.revoke" {
		return nil, false, nil
	}
	if !b.hasBrowserConsent(h.ID, in.Page) {
		return nil, false, nil
	}
	scope, ok := b.cachedBrowserOwner(h, in.Page)
	if !ok {
		return nil, false, nil
	}
	if in.Action == "profiles.allowAgents" {
		var args struct {
			ID      string `json:"id"`
			Allowed bool   `json:"allowed"`
		}
		if err := browserDecode(in.Args, &args); err != nil {
			return nil, true, err
		}
		if args.Allowed {
			return nil, false, nil
		}
		store, err := b.browserPolicies()
		if err != nil {
			return nil, true, err
		}
		b.blockBrowserAgents(h.ID, args.ID)
		policy, err := store.SetEnabled(scope, args.ID, false)
		if err != nil {
			return nil, true, browserPolicyError(err)
		}
		return api.BrowserProfile{ID: args.ID, Name: policy.Name, Agents: false}, true, nil
	}
	value, err := b.revokeBrowserApprovals(h, scope, in.Args)
	return value, true, err
}
func (b *Backend) revokeBrowserApprovals(h Hub, scope browserpolicy.Scope, raw json.RawMessage) (any, error) {
	var args struct {
		ScriptID string `json:"script_id"`
		Version  int64  `json:"version,omitempty"`
	}
	if err := browserDecode(raw, &args); err != nil {
		return nil, err
	}
	if args.ScriptID == "" || args.Version < 0 {
		return nil, browserFailure("invalid_argument", 400)
	}
	store, err := b.browserPolicies()
	if err != nil {
		return nil, err
	}
	profiles, err := store.Profiles(scope)
	if err != nil {
		return nil, browserPolicyError(err)
	}
	for _, p := range profiles {
		relevant := false
		for _, snap := range p.Snapshots {
			if snap.Page == scope.Page && snap.Approval.ScriptID == args.ScriptID && (args.Version == 0 || snap.Approval.Version == args.Version) {
				relevant = true
			}
		}
		if !relevant {
			continue
		}
		b.blockBrowserAgents(h.ID, p.Profile)
		if _, err := store.Revoke(scope, p.Profile, args.ScriptID, args.Version); err != nil {
			return nil, browserPolicyError(err)
		}
		b.unblockBrowserAgents(h.ID, p.Profile)
	}
	if b.Local != nil && b.Store != nil {
		go b.reconcileActionHosts()
	}
	return true, nil
}
func (b *Backend) browserApprovalCall(ctx context.Context, c *client.Client, h Hub, in browserCall) (any, error) {
	scope, err := b.browserOwner(ctx, c, h, in.Page)
	if err != nil {
		return nil, err
	}
	store, err := b.browserPolicies()
	if err != nil {
		return nil, err
	}
	switch in.Action {
	case "approvals.list":
		var args struct{}
		if err := browserDecode(in.Args, &args); err != nil {
			return nil, err
		}
		profiles, err := store.Profiles(scope)
		if err != nil {
			return nil, browserPolicyError(err)
		}
		out := []api.BrowserApproval{}
		for _, p := range profiles {
			for _, snap := range p.Snapshots {
				if snap.Page == scope.Page {
					out = append(out, snap.Approval)
				}
			}
		}
		return out, nil
	case "approvals.revoke":
		return b.revokeBrowserApprovals(h, scope, in.Args)
	case "approvals.review", "approvals.request":
		settings, err := c.HubSettings(ctx)
		if err != nil {
			return nil, err
		}
		if settings.E2E {
			return nil, browserFailure("unsupported", 501)
		}
		var req browserpolicy.Request
		if err := browserDecode(in.Args, &req); err != nil {
			return nil, err
		}
		review, err := browserpolicy.Prepare(scope, req)
		if err != nil {
			return nil, browserPolicyError(err)
		}
		comp := b.browserComponent()
		status := comp.Status()
		if !status.Supported {
			return nil, browserFailure("unsupported", 501)
		}
		if !status.Installed {
			return nil, browserFailure("not_installed", 409)
		}
		if err := comp.EnsureRunning(ctx); err != nil {
			return nil, browserFailure(browserError(err), 409)
		}
		var native api.BrowserStatus
		if err := comp.Call(ctx, browserScope(h.ID, in.Page, in.Instance), "GET", "/status", nil, &native); err != nil {
			return nil, browserFailure(browserError(err), 409)
		}
		for _, p := range native.Profiles {
			if p.ID == req.Profile {
				review.ProfileName = p.Name
			}
		}
		if review.ProfileName == "" {
			return nil, browserFailure("not_found", 404)
		}
		if in.Action == "approvals.review" {
			return review, nil
		}
		bound := req.Profile + "\x00" + review.Digest + "\x00" + scope.Owner + "\x00" + scope.Workspace
		if !b.consumeBrowserConfirmation(in.Confirmation, browserScope(h.ID, in.Page, in.Instance), in.Action, bound) {
			return nil, browserFailure("permission_denied", 403)
		}
		b.blockBrowserAgents(h.ID, req.Profile)
		defer b.unblockBrowserAgents(h.ID, req.Profile)
		approval, _, err := store.ApproveConfirmed(scope, req, time.Now())
		if err != nil {
			return nil, browserPolicyError(err)
		}
		b.unblockBrowserAgents(h.ID, req.Profile)
		if b.Local != nil && b.Store != nil {
			go b.reconcileActionHosts()
		}
		return approval, nil
	}
	return nil, browserFailure("unsupported", 400)
}
func isBrowserApproval(action string) bool { return strings.HasPrefix(action, "approvals.") }
