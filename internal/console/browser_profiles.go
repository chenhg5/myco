package console

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

func (b *Backend) removeBrowserProfile(ctx context.Context, c *client.Client, h Hub, in browserCall, scope string) (any, error) {
	var args api.BrowserProfileRemoveInput
	if browserDecode(in.Args, &args) != nil || !validBrowserID(args.ID, "", 80) {
		return nil, browserFailure("invalid_argument", 400)
	}
	who, err := c.Whoami(ctx)
	if err != nil {
		return nil, err
	}
	if who.Device == nil || !b.consumeBrowserConfirmation(in.Confirmation, scope, in.Action, args.ID) {
		return nil, browserFailure("permission_denied", 403)
	}
	var status api.BrowserStatus
	if err = b.browserComponent().Call(ctx, scope, http.MethodGet, "/status", nil, &status); err != nil {
		return nil, browserComponentFailure(err)
	}
	if !slices.Contains(status.Features, "profile_delete") {
		return nil, browserFailure("unsupported", 409)
	}
	book := b.browserRuns()
	book.mu.Lock()
	busy := book.controlBusy(args.ID)
	for _, r := range book.runs {
		r.mu.Lock()
		busy = busy || r.profile == args.ID && !r.closed && (r.id == "" || r.update.State == "running" || r.update.State == "paused")
		r.mu.Unlock()
	}
	book.mu.Unlock()
	if busy {
		return nil, browserFailure("busy", 409)
	}
	if err = b.browserComponent().Call(ctx, scope, http.MethodDelete, "/profiles/"+url.PathEscape(args.ID), nil, nil); err != nil {
		return nil, browserComponentFailure(err)
	}
	// Native profiles are machine-wide, so their deleted identity must never
	// be advertised from another workspace's stale local policy.
	b.browser.mu.Lock()
	if b.browser.deletedProfiles == nil {
		b.browser.deletedProfiles = map[string]bool{}
	}
	b.browser.deletedProfiles[args.ID] = true
	b.browser.mu.Unlock()
	policies, err := b.browserPolicies()
	if err != nil {
		return nil, err
	}
	lastDedicated := false
	for _, profile := range status.Profiles {
		if profile.ID == args.ID && profile.Kind == api.BrowserAgentProfile {
			lastDedicated = true
		}
	}
	for _, profile := range status.Profiles {
		if profile.ID != args.ID && profile.Kind == api.BrowserAgentProfile {
			lastDedicated = false
			break
		}
	}
	if err = policies.ForgetProfile(args.ID, lastDedicated); err != nil {
		return nil, browserPolicyError(err)
	}
	if b.Local != nil && b.Store != nil {
		go b.reconcileActionHosts()
	}
	return true, nil
}
