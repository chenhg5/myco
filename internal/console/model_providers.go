package console

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/provider"
)

// The member's model providers (i115), as this App manages them. Only
// here, on the person's own device, is a key ever in the clear: it comes
// from the App's form, is sealed for the member's devices before it goes
// to the hub, and is opened again only to test it or to seal it for an
// agent's computer. It is never logged, and errors name only a host.

// ConsoleProvider is a provider as the App shows it: no sealed key, but
// whether this device can open it and its last four characters. Its
// vendor, addresses and small model are the sealed ones when this device
// can open them (the hub's copy is only for showing).
type ConsoleProvider struct {
	api.ModelProvider
	KeyHint     string `json:"key_hint,omitempty"`
	KeyReadable bool   `json:"key_readable"`
	// Rewrapped: this device sealed it again for the member's devices now
	// (one was added or removed).
	Rewrapped bool `json:"rewrapped,omitempty"`
	// SettingsChanged: the hub's copy of its vendor, addresses or small
	// model is not what was sealed. The sealed ones are shown and used; the
	// hub's are not to be trusted.
	SettingsChanged bool `json:"settings_changed,omitempty"`
	// SealAgain: it was sealed before its addresses went into the
	// envelope; it is used only once saved again with its key.
	SealAgain bool `json:"seal_again,omitempty"`
}

// providerSecret is what a provider's envelope holds: the key and every
// setting the key is used with, so that whoever can change the hub's copy
// (the hub itself, its administrator token) cannot send the key elsewhere.
type providerSecret struct {
	APIKey        string `json:"api_key"`
	Vendor        string `json:"vendor"`
	AnthropicBase string `json:"anthropic_base,omitempty"`
	OpenAIBase    string `json:"openai_base,omitempty"`
	SmallModel    string `json:"small_model,omitempty"`
}

// errSealAgain: an envelope from before the addresses were sealed too.
var errSealAgain = &client.Error{Status: 409, Code: "seal_again", Message: "this provider was saved before its addresses were sealed with its key",
	Suggestion: "edit it and enter its key again"}

// same reports whether the hub's copy says what was sealed.
func (s providerSecret) same(p api.ModelProvider) bool {
	return s.Vendor == p.Vendor && s.AnthropicBase == p.AnthropicBase && s.OpenAIBase == p.OpenAIBase && s.SmallModel == p.SmallModel
}

// shown is p with the sealed settings (what is used).
func (s providerSecret) shown(p api.ModelProvider) api.ModelProvider {
	p.Vendor, p.AnthropicBase, p.OpenAIBase, p.SmallModel = s.Vendor, s.AnthropicBase, s.OpenAIBase, s.SmallModel
	return p
}

// ProviderInput is the App's form: the provider and, when it is new or
// changes, the key.
type ProviderInput struct {
	Name          *string `json:"name,omitempty"`
	Vendor        *string `json:"vendor,omitempty"`
	AnthropicBase *string `json:"anthropic_base,omitempty"`
	OpenAIBase    *string `json:"openai_base,omitempty"`
	SmallModel    *string `json:"small_model,omitempty"`
	APIKey        string  `json:"api_key,omitempty"`
}

func (b *Backend) registerModelProviders(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/providers", b.withHub(b.listModelProviders))
	m.HandleFunc("POST /app/hubs/{id}/providers", b.withHub(b.createProvider))
	m.HandleFunc("PATCH /app/hubs/{id}/providers/{pid}", b.withHub(b.updateProvider))
	m.HandleFunc("DELETE /app/hubs/{id}/providers/{pid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.DeleteModelProvider(r.Context(), r.PathValue("pid")); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))
	m.HandleFunc("POST /app/hubs/{id}/providers/test", b.withHubTimeout(30*time.Second, b.testProvider))
	m.HandleFunc("POST /app/hubs/{id}/providers/{pid}/test", b.withHubTimeout(30*time.Second, b.testProvider))
	m.HandleFunc("POST /app/hubs/{id}/providers/{pid}/push", b.withHubTimeout(2*time.Minute, b.pushProvider))
}

// providerMember is the member this device belongs to (providers are
// sealed for their devices).
func providerMember(ctx context.Context, c *client.Client) (string, error) {
	me, err := c.Whoami(ctx)
	if err != nil {
		return "", err
	}
	if me.Member == nil {
		return "", &client.Error{Status: 403, Code: "permission_denied", Message: "model providers belong to members: this App is not signed in as one"}
	}
	return me.Member.ID, nil
}

func (b *Backend) listModelProviders(w http.ResponseWriter, r *http.Request, c *client.Client) {
	list, err := c.ModelProviders(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	member, merr := providerMember(r.Context(), c)
	out := struct {
		E2E       bool              `json:"e2e"`
		Providers []ConsoleProvider `json:"providers"`
	}{E2E: list.E2E, Providers: []ConsoleProvider{}}
	for _, p := range list.Providers {
		cp := ConsoleProvider{ModelProvider: p}
		if merr == nil && list.E2E {
			cp = describe(r.Context(), c, member, p)
		}
		cp.KeySealed = ""
		out.Providers = append(out.Providers, cp)
	}
	writeJSON(w, http.StatusOK, out)
}

// describe opens a provider's envelope (sealing it again for the member's
// devices now when a device was added or removed) and says what it holds.
func describe(ctx context.Context, c *client.Client, member string, p api.ModelProvider) ConsoleProvider {
	cp := ConsoleProvider{ModelProvider: p}
	sec, err := openSecret(ctx, c, member, p)
	if errors.Is(err, errSealAgain) {
		cp.SealAgain = true
		return cp
	}
	if err != nil {
		return cp
	}
	cp.KeyReadable, cp.KeyHint = true, hint(sec.APIKey)
	cp.SettingsChanged = !sec.same(p)
	cp.ModelProvider = sec.shown(p)
	if stale, err := c.ProviderKeyStale(ctx, p); err == nil && stale {
		if body, err := sealSecret(ctx, c, member, p.ID, p.KeyVersion, sec); err == nil {
			v := p.KeyVersion
			if up, err := c.UpdateModelProvider(ctx, p.ID, api.ModelProviderRequest{KeySealed: &body, KeyVersion: &v}); err == nil {
				cp.ModelProvider, cp.Rewrapped = sec.shown(*up), true
			}
		}
	}
	return cp
}

func (b *Backend) createProvider(w http.ResponseWriter, r *http.Request, c *client.Client) {
	var in ProviderInput
	if !decode(w, r, &in) {
		return
	}
	key := strings.TrimSpace(in.APIKey)
	if key == "" || strings.ContainsAny(key, "\r\n\x00") {
		writeErr(w, &client.Error{Status: 400, Code: "invalid_argument", Message: "enter the API key"})
		return
	}
	member, err := providerMember(r.Context(), c)
	if err != nil {
		writeErr(w, err)
		return
	}
	sec := secretFrom(providerSecret{APIKey: key}, in)
	id := "prv_" + strings.ToLower(rand.Text())[:16]
	body, err := sealSecret(r.Context(), c, member, id, 1, sec)
	if err != nil {
		writeErr(w, err)
		return
	}
	one := 1
	req := api.ModelProviderRequest{ID: id, Name: in.Name, Vendor: &sec.Vendor, AnthropicBase: &sec.AnthropicBase, OpenAIBase: &sec.OpenAIBase,
		SmallModel: &sec.SmallModel, KeySealed: &body, KeyVersion: &one}
	p, err := c.CreateModelProvider(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ConsoleProvider{ModelProvider: withoutKey(*p), KeyReadable: true, KeyHint: hint(key)})
}

// updateProvider: a new name alone is the hub's to keep; a new key, vendor,
// address or small model is sealed again (a new key version), since the key
// is only ever sent where its own envelope says.
func (b *Backend) updateProvider(w http.ResponseWriter, r *http.Request, c *client.Client) {
	var in ProviderInput
	if !decode(w, r, &in) {
		return
	}
	id := r.PathValue("pid")
	key := strings.TrimSpace(in.APIKey)
	if strings.ContainsAny(key, "\r\n\x00") {
		writeErr(w, &client.Error{Status: 400, Code: "invalid_argument", Message: "enter the API key on one line"})
		return
	}
	cur, err := findProvider(r.Context(), c, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	member, err := providerMember(r.Context(), c)
	if err != nil {
		writeErr(w, err)
		return
	}
	old, oerr := openSecret(r.Context(), c, member, *cur)
	if key == "" && oerr != nil {
		// (nothing to seal the new settings with: the key is needed)
		if in.Vendor != nil || in.AnthropicBase != nil || in.OpenAIBase != nil || in.SmallModel != nil {
			writeErr(w, &client.Error{Status: 409, Code: "key_unreadable", Message: "enter the key again to change its vendor, addresses or small model",
				Suggestion: "this device cannot open the key it has"})
			return
		}
	}
	req := api.ModelProviderRequest{Name: in.Name}
	sec := old
	if key != "" {
		sec.APIKey = key
	}
	sec = secretFrom(sec, in)
	if key != "" || (oerr == nil && sec != old) {
		v := cur.KeyVersion + 1
		body, err := sealSecret(r.Context(), c, member, id, v, sec)
		if err != nil {
			writeErr(w, err)
			return
		}
		req.Vendor, req.AnthropicBase, req.OpenAIBase, req.SmallModel = &sec.Vendor, &sec.AnthropicBase, &sec.OpenAIBase, &sec.SmallModel
		req.KeySealed, req.KeyVersion = &body, &v
	}
	p, err := c.UpdateModelProvider(r.Context(), id, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := describe(r.Context(), c, member, *p)
	out.KeySealed = ""
	writeJSON(w, http.StatusOK, out)
}

// testProvider reads the vendor's models once with the key: a saved
// provider's (at its sealed address; its list is kept), or the form's
// before it is saved.
func (b *Backend) testProvider(w http.ResponseWriter, r *http.Request, c *client.Client) {
	var in ProviderInput
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	var saved *api.ModelProvider
	var sec providerSecret
	if id := r.PathValue("pid"); id != "" {
		p, err := findProvider(r.Context(), c, id)
		if err != nil {
			writeErr(w, err)
			return
		}
		saved = p
		member, err := providerMember(r.Context(), c)
		if err != nil {
			writeErr(w, err)
			return
		}
		if sec, err = openSecret(r.Context(), c, member, *p); err != nil {
			writeErr(w, unreadable(err))
			return
		}
	} else {
		sec = secretFrom(providerSecret{APIKey: strings.TrimSpace(in.APIKey)}, in)
	}
	ep := provider.Endpoint{Base: sec.AnthropicBase, Anthropic: true, Key: sec.APIKey}
	if ep.Base == "" {
		ep = provider.Endpoint{Base: sec.OpenAIBase, Key: sec.APIKey}
	}
	if ep.Base == "" || ep.Key == "" {
		writeErr(w, &client.Error{Status: 400, Code: "invalid_argument", Message: "an address and a key are needed to test"})
		return
	}
	ms, err := provider.FetchModels(r.Context(), nil, ep)
	if err != nil {
		// (the error names the host and status only)
		writeErr(w, &client.Error{Status: 502, Code: "provider_unreachable", Message: err.Error()})
		return
	}
	models := make([]api.RuntimeModel, 0, len(ms))
	for _, m := range ms {
		models = append(models, api.RuntimeModel{ID: m.ID, Name: m.Name})
	}
	if saved != nil {
		now := time.Now().UTC()
		_, _ = c.UpdateModelProvider(r.Context(), saved.ID, api.ModelProviderRequest{Models: &models, ModelsAt: &now})
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

// pushProvider sends a provider's current key to every agent that has an
// older one (the App offers it after the key changed).
func (b *Backend) pushProvider(w http.ResponseWriter, r *http.Request, c *client.Client) {
	p, sec, err := providerWithSecret(r.Context(), c, r.PathValue("pid"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ov, err := c.Overview(r.Context(), 0)
	if err != nil {
		writeErr(w, err)
		return
	}
	type failure struct {
		Agent string `json:"agent"`
		Error string `json:"error"`
	}
	out := struct {
		Pushed []string  `json:"pushed"`
		Failed []failure `json:"failed"`
	}{Pushed: []string{}, Failed: []failure{}}
	for _, st := range ov.Agents {
		rs := st.Agent.RunSettings
		if rs == nil || rs.ProviderRef == nil || rs.ProviderRef.ID != p.ID || rs.ProviderRef.KeyVersion >= p.KeyVersion {
			continue
		}
		model := rs.Model
		if model == "" && st.Agent.Runtime != nil {
			model = st.Agent.Runtime.Model
		}
		if err := deliverProvider(r.Context(), c, *p, sec, st.Agent, model); err != nil {
			out.Failed = append(out.Failed, failure{Agent: st.Agent.Name, Error: errText(err)})
			continue
		}
		out.Pushed = append(out.Pushed, st.Agent.Name)
	}
	writeJSON(w, http.StatusOK, out)
}

// useProviderFor sends a provider to an agent's computer, sealed for the
// agent (PUT /app/hubs/{id}/agents/{name}/provider with "workspace").
func useProviderFor(ctx context.Context, c *client.Client, name, id, model string) (*api.Agent, error) {
	p, sec, err := providerWithSecret(ctx, c, id)
	if err != nil {
		return nil, err
	}
	a, err := c.GetAgent(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := deliverProvider(ctx, c, *p, sec, *a, model); err != nil {
		return nil, err
	}
	return c.GetAgent(ctx, name)
}

// providerWithSecret is a provider and what its envelope holds.
func providerWithSecret(ctx context.Context, c *client.Client, id string) (*api.ModelProvider, providerSecret, error) {
	p, err := findProvider(ctx, c, id)
	if err != nil {
		return nil, providerSecret{}, err
	}
	member, err := providerMember(ctx, c)
	if err != nil {
		return nil, providerSecret{}, err
	}
	sec, err := openSecret(ctx, c, member, *p)
	if err != nil {
		return nil, providerSecret{}, unreadable(err)
	}
	return p, sec, nil
}

func unreadable(err error) error {
	if errors.Is(err, errSealAgain) {
		return errSealAgain
	}
	return &client.Error{Status: 409, Code: "key_unreadable", Message: "this device cannot open the provider's key yet",
		Suggestion: "open the App on another of your devices once, or enter the key again here"}
}

// deliverProvider seals the provider for one agent and records which
// provider and key version it has.
func deliverProvider(ctx context.Context, c *client.Client, p api.ModelProvider, sec providerSecret, a api.Agent, model string) error {
	cli := a.Binding.Harness
	if a.Runtime != nil && a.Runtime.CLI != "" {
		cli = a.Runtime.CLI
	}
	raw, err := manualFor(sec, cli, model)
	if err != nil {
		return err
	}
	sealed, err := c.SealProvider(ctx, a.Name, raw)
	if err != nil {
		return err
	}
	custom := "custom"
	_, err = c.PatchRunSettings(ctx, a.Name, api.RunSettingsPatch{Provider: &custom, ProviderEncrypted: sealed,
		ProviderRef: &api.ProviderRef{ID: p.ID, KeyVersion: p.KeyVersion}})
	return err
}

// manualFor is the custom provider an agent of cli gets: the sealed
// address for that CLI, the key, the model and the small model.
func manualFor(sec providerSecret, cli, model string) ([]byte, error) {
	m := provider.Manual{APIKey: sec.APIKey, Model: strings.TrimSpace(model), SmallModel: sec.SmallModel}
	switch cli {
	case "claude":
		m.BaseURL = sec.AnthropicBase
		if m.SmallModel == "" {
			if v, ok := provider.VendorByID(sec.Vendor); ok {
				m.SmallModel = v.Small
			}
		}
	case "codex":
		m.BaseURL, m.WireAPI, m.SmallModel = sec.OpenAIBase, "chat", ""
	}
	if m.BaseURL == "" {
		return nil, &client.Error{Status: 400, Code: "invalid_argument", Message: "this provider has no address for " + cli}
	}
	if err := m.Validate(cli); err != nil {
		return nil, &client.Error{Status: 400, Code: "invalid_argument", Message: err.Error()}
	}
	return json.Marshal(m)
}

// sealedSpawn seals the chosen provider for a computer's machine node
// when it can take one (sealed_spawn and its key registered); false when it
// cannot, and the provider is sent to the new agent after it joins.
func sealedSpawn(ctx context.Context, c *client.Client, machineID string, in *api.SpawnRequest, providerID string) (p *api.ModelProvider, sec providerSecret, sealed bool, err error) {
	p, sec, err = providerWithSecret(ctx, c, providerID)
	if err != nil {
		return nil, sec, false, err
	}
	raw, err := manualFor(sec, in.CLI, in.Model)
	if err != nil {
		return nil, sec, false, err
	}
	machines, err := c.Machines(ctx)
	if err != nil {
		return nil, sec, false, err
	}
	node := ""
	for _, m := range machines {
		if m.ID == machineID {
			node = m.Node
		}
	}
	if node == "" {
		return p, sec, false, nil // an older hub: after the agent joins
	}
	if a, err := c.GetAgent(ctx, node); err != nil || a.Runtime == nil || !slices.Contains(a.Runtime.Features, "sealed_spawn") {
		return p, sec, false, nil // an older machine node
	}
	enc, err := c.SealProvider(ctx, node, raw)
	if err != nil {
		var ce *client.Error
		if errors.As(err, &ce) && ce.Code == "keys_missing" {
			return p, sec, false, nil // its key not registered yet
		}
		return nil, sec, false, err
	}
	in.ProviderEncrypted, in.ProviderRef = enc, &api.ProviderRef{ID: p.ID, KeyVersion: p.KeyVersion}
	return p, sec, true, nil
}

// deliverAfterJoin sends the provider to an agent that joined without it
// (an older machine): once its runner has registered its key, before
// anything is sent to it.
func deliverAfterJoin(ctx context.Context, c *client.Client, p api.ModelProvider, sec providerSecret, agent, model string) error {
	for {
		keys, err := c.KeysOf(ctx, agent)
		if err == nil && len(keys.Keys[agent]) == 1 {
			a, err := c.GetAgent(ctx, agent)
			if err != nil {
				return err
			}
			return deliverProvider(ctx, c, p, sec, *a, model)
		}
		select {
		case <-ctx.Done():
			return &client.Error{Status: 504, Code: "keys_missing", Message: "the new agent did not register its encryption key in time",
				Suggestion: "choose the provider again in the agent's details once its runner is up"}
		case <-time.After(2 * time.Second):
		}
	}
}

func findProvider(ctx context.Context, c *client.Client, id string) (*api.ModelProvider, error) {
	list, err := c.ModelProviders(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list.Providers {
		if list.Providers[i].ID == id {
			return &list.Providers[i], nil
		}
	}
	return nil, &client.Error{Status: 404, Code: "not_found", Message: "model provider not found"}
}

// openSecret opens a provider's envelope. One sealed before its addresses
// were is not used (errSealAgain): the hub's addresses cannot be trusted.
func openSecret(ctx context.Context, c *client.Client, member string, p api.ModelProvider) (providerSecret, error) {
	raw, err := c.OpenProviderKey(ctx, member, p)
	if err != nil {
		return providerSecret{}, err
	}
	var s providerSecret
	if json.Unmarshal(raw, &s) != nil || s.APIKey == "" {
		return providerSecret{}, errors.New("unreadable provider key")
	}
	if s.Vendor == "" || (s.AnthropicBase == "" && s.OpenAIBase == "") {
		return providerSecret{}, errSealAgain
	}
	return s, nil
}

func sealSecret(ctx context.Context, c *client.Client, member, id string, version int, s providerSecret) (string, error) {
	raw, _ := json.Marshal(s)
	return c.SealProviderKey(ctx, member, id, version, raw)
}

// secretFrom is s with what the form changes: a catalogue vendor's
// addresses come with it when none are given.
func secretFrom(s providerSecret, in ProviderInput) providerSecret {
	if in.Vendor != nil {
		s.Vendor = strings.TrimSpace(*in.Vendor)
		if v, ok := provider.VendorByID(s.Vendor); ok && len(v.Hosts) > 0 {
			origin := "https://" + v.Hosts[0]
			if in.AnthropicBase == nil {
				s.AnthropicBase = origin + v.Anthropic
			}
			if in.OpenAIBase == nil {
				s.OpenAIBase = origin + v.OpenAI
			}
		}
	}
	if in.AnthropicBase != nil {
		s.AnthropicBase = strings.TrimSpace(*in.AnthropicBase)
	}
	if in.OpenAIBase != nil {
		s.OpenAIBase = strings.TrimSpace(*in.OpenAIBase)
	}
	if in.SmallModel != nil {
		s.SmallModel = strings.TrimSpace(*in.SmallModel)
	}
	return s
}

func withoutKey(p api.ModelProvider) api.ModelProvider { p.KeySealed = ""; return p }

// hint is a key's last four characters, for "···a1b2" (nothing for a
// short one).
func hint(key string) string {
	if len(key) < 12 {
		return ""
	}
	return key[len(key)-4:]
}
