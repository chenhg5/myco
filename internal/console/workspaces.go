package console

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/harness/machinenode"
)

// Workspaces of one hub server (v0.19.9+): the hub is the default workspace
// and serves others at <hub>/w/<name>. The app adds each as a hub of its
// own; these routes create them from the app and move a machine that is
// already on the server into another of its workspaces.
//
//	GET  /app/hubs/{id}/workspaces            {supported, allowed, server, workspaces}
//	POST /app/hubs/{id}/workspaces            {name} → the new workspace, added to the app
//	GET  /app/hubs/{id}/server-machines       machines of the same server's other workspaces
//	POST /app/hubs/{id}/machines/add-from     {from_hub, machine_id}: that machine joins this one

func (b *Backend) workspaceRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/workspaces", b.listWorkspaces)
	m.HandleFunc("POST /app/hubs/{id}/workspaces", b.createWorkspace)
	m.HandleFunc("GET /app/hubs/{id}/server-machines", b.serverMachines)
	m.HandleFunc("POST /app/hubs/{id}/machines/add-from", b.addMachineFrom)
}

// serverRoot is a hub URL without its workspace path.
func serverRoot(u string) string {
	if i := strings.Index(u, "/w/"); i > 0 {
		return u[:i]
	}
	return strings.TrimRight(u, "/")
}

// sameServer: hub URLs on one server (scheme, host, port).
func sameServer(a, b string) bool { return machinenode.SameServer(a, b) }

// serverClient is a client for the server's own address with the token of
// the app's entry there (the default workspace), if the app has one; else
// with hub's own token (it is then the default workspace itself, or the
// server refuses).
func (b *Backend) serverClient(hub Hub) (*client.Client, Hub, error) {
	root := serverRoot(hub.URL)
	owner := hub
	if hub.URL != root {
		if hubs, err := b.Store.List(); err == nil {
			for _, h := range hubs {
				if strings.TrimRight(h.URL, "/") == root {
					owner = h
				}
			}
		}
	}
	tok, err := b.Secrets.Get(owner.ID)
	if err != nil {
		return nil, owner, &client.Error{Code: "not_configured", Message: "no console token for " + owner.Name}
	}
	nc := b.NewClient
	if nc == nil {
		nc = client.New
	}
	c, err := nc(root, tok)
	return c, owner, err
}

func (b *Backend) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	hub, err := b.find(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	out := map[string]any{"supported": false, "allowed": false, "server": serverRoot(hub.URL), "workspaces": []api.Workspace{}}
	c, _, err := b.serverClient(hub)
	if err != nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), b.timeout())
	defer cancel()
	list, err := c.Workspaces(ctx)
	var ce *client.Error
	switch {
	case err == nil:
		out["supported"], out["allowed"], out["workspaces"] = true, true, list
	case errors.As(err, &ce) && (ce.Status == http.StatusForbidden || ce.Status == http.StatusUnauthorized):
		out["supported"] = true // a newer hub; this app is not its server's owner
	}
	writeJSON(w, http.StatusOK, out)
}

func (b *Backend) createWorkspace(w http.ResponseWriter, r *http.Request) {
	hub, err := b.find(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, _, err := b.serverClient(hub)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*b.timeout())
	defer cancel()
	ws, err := c.CreateWorkspace(ctx, strings.TrimSpace(in.Name), false)
	if err != nil {
		writeErr(w, err)
		return
	}
	if ws.OwnerLink == nil {
		writeErr(w, &client.Error{Status: http.StatusBadGateway, Code: "internal", Message: "the hub made the workspace but gave no link to it",
			Suggestion: "on the server: sudo myco console link --hub " + ws.URL})
		return
	}
	u, tok, err := ParseAppLink(ws.OwnerLink.Link)
	if err != nil {
		writeErr(w, err)
		return
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		name = ws.Name
	}
	st, code, err := b.addHubWith(ctx, name, u, tok)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, code, st)
}

// ServerMachine is a machine of another workspace of the same server.
type ServerMachine struct {
	HubID     string `json:"hub_id"`
	HubName   string `json:"hub_name"`
	MachineID string `json:"machine_id"`
	Name      string `json:"name"`
	Hostname  string `json:"hostname,omitempty"`
	OS        string `json:"os,omitempty"`
	Online    bool   `json:"online"`
	CanSpawn  bool   `json:"can_spawn"`
}

// serverMachines lists the machines of the other workspaces (in this app)
// on hub's server that are not in hub yet.
func (b *Backend) serverMachines(w http.ResponseWriter, r *http.Request) {
	hub, err := b.find(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	hubs, err := b.Store.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), b.timeout())
	defer cancel()
	have := map[string]bool{}
	if c, err := b.client(hub); err == nil {
		if ms, err := c.Machines(ctx); err == nil {
			for _, m := range ms {
				have[machineKey(m)] = true
			}
		}
	}
	out := []ServerMachine{}
	seen := map[string]bool{}
	for _, h := range hubs {
		if h.ID == hub.ID || !sameServer(h.URL, hub.URL) {
			continue
		}
		c, err := b.client(h)
		if err != nil {
			continue
		}
		ms, err := c.Machines(ctx)
		if err != nil {
			continue
		}
		for _, m := range ms {
			k := machineKey(m)
			if m.Legacy || have[k] || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, ServerMachine{HubID: h.ID, HubName: h.Name, MachineID: m.ID, Name: m.Name,
				Hostname: m.Hostname, OS: m.OS, Online: m.Online, CanSpawn: m.CanSpawn})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"machines": out})
}

// machineKey tells the same computer apart across workspaces: its host
// name when known, else its machine name.
func machineKey(m api.Machine) string {
	if m.Hostname != "" {
		return "h:" + strings.ToLower(m.Hostname)
	}
	return "n:" + strings.ToLower(m.Name)
}

// addMachineFrom: a machine of another workspace of this server joins this
// one, through its machine node there (no command on the machine).
func (b *Backend) addMachineFrom(w http.ResponseWriter, r *http.Request) {
	hub, err := b.find(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		FromHub   string `json:"from_hub"`
		MachineID string `json:"machine_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	from, err := b.find(in.FromHub)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !sameServer(from.URL, hub.URL) {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument",
			Message: "that machine is on another server", Suggestion: "join it with a link from this workspace instead"})
		return
	}
	target, err := b.client(hub)
	if err != nil {
		writeErr(w, err)
		return
	}
	source, err := b.client(from)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	link, err := target.CreateEnrollment(ctx, api.CreateEnrollmentRequest{Kind: "link", MaxUses: 1, TTLSeconds: 600,
		Note: "added from workspace " + from.Name}, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	if link.JoinURL == "" {
		writeErr(w, &client.Error{Status: http.StatusBadGateway, Code: "internal", Message: "the workspace made no join link"})
		return
	}
	res, err := source.SpawnOnMachine(ctx, in.MachineID, api.SpawnRequest{JoinLink: link.JoinURL}, 2*time.Minute)
	if err != nil {
		writeErr(w, err)
		return
	}
	if res.Denied || res.Error != "" || res.Joined == "" {
		msg := res.Error
		if msg == "" {
			msg = "the machine did not join"
		}
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "invalid_state", Message: msg,
			Suggestion: "the machine needs myco v0.19.9 or newer (myco upgrade); or join it with this workspace's link: myco machine join --link <link>"})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
