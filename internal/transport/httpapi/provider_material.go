package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

// Keys the hub keeps (i119, a workspace with end-to-end encryption off)
// leave it only here: to the one runner or machine node a provider was
// set for (GET /v1/me/provider-material), and to the provider's owner (GET
// /v1/console/providers/{id}/key). Every answer is recorded (who, which
// provider, which version, never the key), each caller is held to a few a
// minute, and nothing here is cached or logged.

// secretReads limits how often one caller reads keys.
const secretReadsPerMinute = 6

type rateLimit struct {
	mu    sync.Mutex
	every time.Duration
	max   int
	seen  map[string][]time.Time
}

func newRateLimit(max int, every time.Duration) *rateLimit {
	return &rateLimit{every: every, max: max, seen: map[string][]time.Time{}}
}

// allow records one call by who, and reports whether it is within the limit.
func (l *rateLimit) allow(who string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, ts := range l.seen {
		kept := ts[:0]
		for _, t := range ts {
			if now.Sub(t) < l.every {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(l.seen, k)
		} else {
			l.seen[k] = kept
		}
	}
	if len(l.seen[who]) >= l.max {
		return false
	}
	l.seen[who] = append(l.seen[who], now)
	return true
}

// A spawnGrant lets one machine node fetch, once, the provider a person
// started a session on it with (i119): made for that spawn request alone,
// and gone when the request ends or after grantTTL.
type spawnGrant struct {
	node     domain.AgentName
	provider string
	owner    string // the provider's owner (member id)
	from     string // "@console:<owner name>"
	cli      string
	model    string
	expires  time.Time
	used     bool
}

const grantTTL = 10 * time.Minute

type spawnGrants struct {
	mu sync.Mutex
	m  map[string]*spawnGrant
}

func newSpawnGrants() *spawnGrants { return &spawnGrants{m: map[string]*spawnGrant{}} }

func (g *spawnGrants) add(x *spawnGrant, now time.Time) string {
	id := "pgr_" + rand.Text()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweep(now)
	x.expires = now.Add(grantTTL)
	g.m[id] = x
	return id
}

// take answers the grant id for node, once, while it is valid.
func (g *spawnGrants) take(id string, node domain.AgentName, now time.Time) (*spawnGrant, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweep(now)
	x, ok := g.m[id]
	if !ok || x.used || x.node != node {
		return nil, false
	}
	x.used = true
	return x, true
}

func (g *spawnGrants) remove(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.m, id)
}

func (g *spawnGrants) sweep(now time.Time) {
	for id, x := range g.m {
		if !now.Before(x.expires) {
			delete(g.m, id)
		}
	}
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

// providerMaterial gives a runner its agent's provider the hub keeps
// (?id=<request>), or a machine node the provider of the session it is
// starting (?grant=<grant>).
func (s *Server) providerMaterial(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	noStore(w)
	if s.modelProviders == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "this hub does not keep providers"))
		return
	}
	// Held to a few a minute: refusals, and reads for a request id (a
	// runner reads one once, again only when its receipt was lost). A
	// grant is its own limit: made by a person for one spawn, used once.
	limited := func() bool {
		if s.secretReads.allow("agent:"+string(a.Name), time.Now()) {
			return false
		}
		writeError(w, domain.Errf(domain.CodeRateLimited, "too many provider reads: try again in a minute"))
		return true
	}
	id, grant := r.URL.Query().Get("id"), r.URL.Query().Get("grant")
	switch {
	case grant != "":
		x, ok := s.grants.take(grant, a.Name, time.Now())
		if !ok {
			s.record(r, "provider.material.refused", string(a.Name), "grant")
			if limited() {
				return
			}
			writeError(w, domain.Errf(domain.CodePermissionDenied, "no such provider grant for this node (each is for one request, once, for 10 minutes)"))
			return
		}
		m, version, err := s.modelProviders.Material(r.Context(), x.provider, x.owner, x.cli, x.model)
		if err != nil {
			writeError(w, err)
			return
		}
		m.ID, m.From = grant, x.from
		s.recordAs(r, app.Actor{Agent: a}, "provider.material", string(a.Name), "provider="+x.provider+" key_version="+strconv.Itoa(version)+" spawn")
		writeJSON(w, http.StatusOK, m)
	case id != "":
		if limited() {
			return
		}
		hp := a.RunSettings.ProviderHub
		if hp == nil || hp.ID != id {
			s.record(r, "provider.material.refused", string(a.Name), "request")
			writeError(w, domain.Errf(domain.CodeNotFound, "no such provider request for this agent"))
			return
		}
		// Set by the agent's owner only (the hub checked when it was set;
		// checked again, as the owner may have changed since).
		ownerName := s.memberName(r.Context(), a.Owner)
		if a.Owner == "" || ownerName == "" || hp.From != app.ConsoleSenderPrefix+ownerName {
			s.record(r, "provider.material.refused", string(a.Name), "not its owner's")
			writeError(w, domain.Errf(domain.CodePermissionDenied, "this provider was not set by the agent's owner"))
			return
		}
		var m api.ProviderMaterial
		detail := ""
		if hp.Ref.ID != "" {
			got, version, err := s.modelProviders.Material(r.Context(), hp.Ref.ID, a.Owner, a.Runtime.CLI, hp.Model)
			if err != nil {
				writeError(w, err)
				return
			}
			m, detail = got, "provider="+hp.Ref.ID+" key_version="+strconv.Itoa(version)
		} else {
			raw, err := s.modelProviders.OpenForAgent(string(a.Name), hp.ID, hp.Sealed)
			if err != nil {
				writeError(w, err)
				return
			}
			var man api.HubManual
			if json.Unmarshal(raw, &man) != nil {
				writeError(w, domain.Errf(domain.CodeInvalidState, "the provider the hub kept for this agent is damaged"))
				return
			}
			m = api.ProviderMaterial{BaseURL: man.BaseURL, APIKey: man.APIKey, Model: man.Model, WireAPI: man.WireAPI, SmallModel: man.SmallModel}
			detail = "custom"
		}
		m.ID, m.From = hp.ID, hp.From
		s.recordAs(r, app.Actor{Agent: a}, "provider.material", string(a.Name), detail)
		writeJSON(w, http.StatusOK, m)
	default:
		if limited() {
			return
		}
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "id or grant is required"))
	}
}

// modelProviderKey gives a provider's owner the key the hub keeps for it.
func (s *Server) modelProviderKey(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	member, ok := s.providerMember(w, r)
	if !ok {
		return
	}
	if !s.secretReads.allow("member:"+member, time.Now()) {
		writeError(w, domain.Errf(domain.CodeRateLimited, "too many key reads: try again in a minute"))
		return
	}
	key, version, err := s.modelProviders.Key(r.Context(), member, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "provider.key.read", r.PathValue("id"), "key_version="+strconv.Itoa(version))
	writeJSON(w, http.StatusOK, api.ProviderKey{APIKey: key, KeyVersion: version})
}
