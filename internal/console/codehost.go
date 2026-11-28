package console

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/oscap"
)

type codeHostInput struct {
	Page     osPage `json:"page"`
	Provider string `json:"provider"`
	Host     string `json:"host"`
	Source   string `json:"source"`
	Name     string `json:"name"`
}
type codeHostChallenge struct {
	Input   codeHostInput
	Scope   string
	Private bool
	Until   time.Time
}
type codeHostState struct {
	mu      sync.Mutex
	pending map[string]codeHostChallenge
}

func (b *Backend) codeHostState() *codeHostState {
	b.codeHostOnce.Do(func() { b.codeHost = &codeHostState{pending: map[string]codeHostChallenge{}} })
	return b.codeHost
}
func (b *Backend) codeHostEnv(page osPage) (*oscap.Env, *oscap.Error) {
	if page.ID == "" || page.Hub == "" || b.Secrets == nil {
		return nil, &oscap.Error{Code: oscap.CodeInvalid, Message: "a workspace, account and page are required"}
	}
	credential, err := b.Secrets.Get(page.Hub)
	if err != nil || credential == "" {
		return nil, &oscap.Error{Code: oscap.CodePermission, Message: "sign in to this workspace again"}
	}
	env, _ := b.osInit()
	scoped := *env
	hash := sha256.Sum256([]byte(credential))
	profile := ""
	if b.Store != nil {
		profile = filepath.Clean(b.Store.path)
	}
	scoped.Scope = profile + "\x00" + page.Hub + "\x00" + hex.EncodeToString(hash[:16]) + "\x00" + page.ID
	return &scoped, nil
}
func (b *Backend) codeHostAllowed(w http.ResponseWriter, r *http.Request, in codeHostInput) bool {
	raw, _ := json.Marshal(map[string]string{"action": "connect", "provider": in.Provider, "host": in.Host})
	return b.osAllowed(w, r, osCallIn{Name: "forge.accounts", Page: in.Page, Args: raw})
}
func (b *Backend) codeHostRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /app/codehost/prepare", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Myco-Codehost-Owner") != "1" {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "account connections require the host dialog"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		var in codeHostInput
		if !decode(w, r, &in) {
			return
		}
		if in.Source != "cli" && in.Source != "token" {
			writeOSErr(w, &oscap.Error{Code: oscap.CodeInvalid, Message: "choose CLI login or access token"})
			return
		}
		if !b.codeHostAllowed(w, r, in) {
			return
		}
		env, err := b.codeHostEnv(in.Page)
		if err != nil {
			writeOSErr(w, err)
			return
		}
		origin, private, previewErr := oscap.ForgePreview(r.Context(), in.Provider, in.Host)
		if previewErr != nil {
			writeOSErr(w, oscap.AsError(previewErr))
			return
		}
		in.Host = origin
		state := b.codeHostState()
		state.mu.Lock()
		defer state.mu.Unlock()
		for id, c := range state.pending {
			if time.Now().After(c.Until) {
				delete(state.pending, id)
			}
		}
		if len(state.pending) >= 64 {
			writeOSErr(w, &oscap.Error{Code: oscap.CodeTooLarge, Message: "too many pending account connections"})
			return
		}
		id := rand.Text()
		state.pending[id] = codeHostChallenge{Input: in, Scope: env.Scope, Private: private, Until: time.Now().Add(2 * time.Minute)}
		writeJSON(w, 200, api.CodeHostConfirmation{Confirmation: id, Provider: in.Provider, Host: origin, Source: in.Source, PrivateNetwork: private})
	})
	m.HandleFunc("POST /app/codehost/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Myco-Codehost-Owner") != "1" {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "account connections require the host dialog"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		var in api.CodeHostConnect
		if !decode(w, r, &in) {
			return
		}
		state := b.codeHostState()
		state.mu.Lock()
		c, ok := state.pending[in.Confirmation]
		delete(state.pending, in.Confirmation)
		state.mu.Unlock()
		if !ok || time.Now().After(c.Until) {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "account confirmation expired; connect again"})
			return
		}
		if !b.codeHostAllowed(w, r, c.Input) {
			return
		}
		env, err := b.codeHostEnv(c.Input.Page)
		if err != nil {
			writeOSErr(w, err)
			return
		}
		if env.Scope != c.Scope {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "workspace account changed; connect again"})
			return
		}
		if c.Private && !in.AllowPrivate {
			writeOSErr(w, &oscap.Error{Code: oscap.CodePermission, Message: "private-network access was not confirmed"})
			return
		}
		args, _ := json.Marshal(map[string]any{"action": "connect", "provider": c.Input.Provider, "host": c.Input.Host, "name": c.Input.Name, "source": c.Input.Source, "token": in.Token, "allow_private": c.Private && in.AllowPrivate})
		result, callErr := oscap.Call(r.Context(), env, "forge.accounts", args)
		if callErr != nil {
			writeOSErr(w, callErr)
			return
		}
		writeJSON(w, 200, result)
	})
}
