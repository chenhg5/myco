package console

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/oscap"
)

var credentialDiscoverSources = oscap.CredentialSources

type credentialInput struct {
	Page       osPage                   `json:"page"`
	Descriptor api.CredentialDescriptor `json:"descriptor"`
}
type credentialChallenge struct {
	Input   credentialInput
	Scope   string
	Private bool
	Sources []api.CredentialSource
	Until   time.Time
}
type credentialState struct {
	mu      sync.Mutex
	pending map[string]credentialChallenge
}

func (b *Backend) credentialState() *credentialState {
	b.credentialOnce.Do(func() { b.credential = &credentialState{pending: map[string]credentialChallenge{}} })
	return b.credential
}
func (b *Backend) credentialEnv(ctx context.Context, page osPage) (*oscap.Env, *oscap.Error) {
	if page.ID == "" || page.Hub == "" || b.Secrets == nil {
		return nil, &oscap.Error{Code: oscap.CodeInvalid, Message: "a workspace, account and page are required"}
	}
	credential, err := b.Secrets.Get(page.Hub)
	if err != nil || credential == "" {
		return nil, &oscap.Error{Code: oscap.CodePermission, Message: "sign in to this workspace again"}
	}
	d := b.declared(ctx, page.Hub, page.ID)
	if !d.known || !d.app || d.appID == "" {
		return nil, &oscap.Error{Code: oscap.CodePermission, Message: "application identity could not be verified"}
	}
	env, _ := b.osInit()
	scoped := *env
	hash := sha256.Sum256([]byte(credential))
	profile := ""
	if b.Store != nil {
		profile = filepath.Clean(b.Store.path)
	}
	scoped.Scope = profile + "\x00" + page.Hub + "\x00" + hex.EncodeToString(hash[:16]) + "\x00" + page.ID + "\x00" + d.appID
	return &scoped, nil
}
func (b *Backend) credentialAllowed(w http.ResponseWriter, r *http.Request, in credentialInput) bool {
	return b.osAllowed(w, r, osCallIn{Name: "credentials.connections", Page: in.Page, Args: json.RawMessage(`{"action":"connect"}`)})
}
func (b *Backend) credentialRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /app/credentials/prepare", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Myco-Credential-Owner") != "1" {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "connections require the host dialog"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		var in credentialInput
		if !decode(w, r, &in) {
			return
		}
		if !b.credentialAllowed(w, r, in) {
			return
		}
		env, scopeErr := b.credentialEnv(r.Context(), in.Page)
		if scopeErr != nil {
			writeOSErr(w, scopeErr)
			return
		}
		d, err := oscap.CredentialDescriptorValid(in.Descriptor)
		if err != nil {
			writeOSErr(w, oscap.AsError(err))
			return
		}
		in.Descriptor = d
		private, err := oscap.CredentialPreview(r.Context(), d.Origin)
		if err != nil {
			writeOSErr(w, oscap.AsError(err))
			return
		}
		sources := []api.CredentialSource{}
		for _, s := range credentialDiscoverSources(r.Context(), env) {
			if s.Origin != d.Origin || d.Authentication != "bearer" {
				continue
			}
			valid := true
			for _, p := range d.Paths {
				if s.BasePath != "" && !strings.HasPrefix(p.Path, s.BasePath+"/") {
					valid = false
				}
			}
			if valid {
				sources = append(sources, s)
			}
		}
		state := b.credentialState()
		state.mu.Lock()
		defer state.mu.Unlock()
		for id, c := range state.pending {
			if time.Now().After(c.Until) {
				delete(state.pending, id)
			}
		}
		if len(state.pending) >= 64 {
			writeOSErr(w, &oscap.Error{Code: oscap.CodeTooLarge, Message: "too many pending connections"})
			return
		}
		id := rand.Text()
		state.pending[id] = credentialChallenge{Input: in, Scope: env.Scope, Private: private, Sources: sources, Until: time.Now().Add(2 * time.Minute)}
		// Verification here describes a known address, not an authenticated user.
		verified := d.Origin == "https://api.github.com" || d.Origin == "https://gitlab.com" || len(sources) > 0
		writeJSON(w, 200, api.CredentialConfirmation{Confirmation: id, CredentialDescriptor: d, PrivateNetwork: private, Verified: verified, Sources: sources})
	})
	m.HandleFunc("POST /app/credentials/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Myco-Credential-Owner") != "1" {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "connections require the host dialog"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		var in api.CredentialConnect
		if !decode(w, r, &in) {
			return
		}
		state := b.credentialState()
		state.mu.Lock()
		c, ok := state.pending[in.Confirmation]
		delete(state.pending, in.Confirmation)
		state.mu.Unlock()
		if !ok || time.Now().After(c.Until) {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "confirmation expired; connect again"})
			return
		}
		if !b.credentialAllowed(w, r, c.Input) {
			return
		}
		env, scopeErr := b.credentialEnv(r.Context(), c.Input.Page)
		if scopeErr != nil {
			writeOSErr(w, scopeErr)
			return
		}
		if env.Scope != c.Scope {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "workspace identity changed; connect again"})
			return
		}
		if c.Private && !in.AllowPrivate {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "private-network access was not confirmed"})
			return
		}
		token, source, base := in.Token, "token", ""
		if in.Source != "" {
			found := false
			for _, s := range c.Sources {
				if s.ID != in.Source {
					continue
				}
				found = true
				// Revalidate the host-owned mapping immediately before reading any secret.
				origin, bp, err := oscap.CredentialSourceTarget(s.CLI, s.Host)
				if err != nil || origin != c.Input.Descriptor.Origin || bp != s.BasePath {
					writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "credential destination mismatch"})
					return
				}
				token, err = oscap.CredentialSourceToken(r.Context(), env, s)
				if err != nil {
					writeOSErr(w, oscap.AsError(err))
					return
				}
				source, base = s.CLI, bp
				break
			}
			if !found {
				writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "choose a discovered credential source"})
				return
			}
		}
		result, err := oscap.CredentialStore(env, c.Input.Descriptor, token, source, base, c.Private && in.AllowPrivate)
		if err != nil {
			writeOSErr(w, oscap.AsError(err))
			return
		}
		writeJSON(w, 200, result)
	})
}
