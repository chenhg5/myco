package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

// What a page may use of this computer: the groups of myco.os capabilities
// (as manifests declare them: domain.TemplateOSGroups), and what the person
// allowed. A call in a group not allowed yet answers consent_required; the
// App asks the person (the page cannot: it only talks to the App) and tells
// this backend what they said, then calls again.
//
//	GET    /app/os/grants                         what is allowed, page by page
//	POST   /app/os/grants  {hub, page, name, group, host?, answer: once | always | deny}
//	DELETE /app/os/grants?hub=&page=&group=       take it back (group empty: all of the page's)
//
// "always" is kept in app-grants.json next to the App's hubs; "once" lasts
// until the App quits; "deny" refuses the group until then. Files the person
// picked (fs.pick, fs.saveAs) are the page's to read and write without the
// files groups: picking them was the person's say.

// osGroupOf is the group a capability belongs to ("" for those any page may
// use: the list of capabilities, writing the clipboard).
func osGroupOf(name string) string {
	switch name {
	case "fs.paths", "fs.list", "fs.stat", "fs.read", "fs.search", "fs.scan", "fs.watch", "fs.thumbnail", "fs.pick", "fs.saveAs", "fs.reveal":
		return "files"
	case "fs.open":
		// (opening a file with its app can start a program: as running one,
		// unless it is a file the person picked)
		return "exec"
	case "fs.write", "fs.mkdir", "fs.move", "fs.copy":
		return "files.write"
	case "fs.delete", "fs.trash":
		return "files.delete"
	case "exec.run", "exec.spawn", "process.kill":
		return "exec"
	case "screen.capture", "screen.record", "screen.region":
		return "screen"
	case "audio.sources", "audio.record":
		return "microphone"
	case "speech.status", "speech.install", "speech.transcribe", "speech.diarize":
		return "speech"
	case "clipboard.read":
		return "clipboard"
	case "notify":
		return "notify"
	case "net.fetch":
		return "network"
	case "ssh.profiles", "ssh.run":
		return "ssh"
	case "proxy.profile", "proxy.control":
		return "proxy"
	case "clipboard.write":
		return ""
	}
	switch {
	case strings.HasPrefix(name, "system."):
		return "system"
	case strings.HasPrefix(name, "browser."):
		return "browser"
	case strings.HasPrefix(name, "db."):
		return "database"
	}
	return "system" // (one this list does not know yet)
}

// The person picking a file is their say for these: no need to ask.
var pickFree = map[string]bool{"fs.pick": true, "fs.saveAs": true}

// Groups that may only be allowed for this run of the App, never always.
var osOnceOnly = map[string]bool{"exec": true, "files.delete": true, "screen": true, "browser": true}

type osGrant struct {
	Hub    string   `json:"hub"`
	Page   string   `json:"page"`
	Name   string   `json:"name,omitempty"` // the page's name when allowed, to show
	Groups []string `json:"groups"`
	Hosts  []string `json:"hosts,omitempty"` // network: hosts allowed beyond its manifest's
	At     int64    `json:"at"`
}

type osPerm struct {
	once, always, denied map[string]bool
	hosts, onceHosts     map[string]bool // network: allowed always (kept), for this run
	picked               []string
	name                 string
}

type osGrants struct {
	mu    sync.Mutex
	file  string
	pages map[string]*osPerm // hub \x00 page
	decl  map[string]osDecl
}

// osDecl is what a page's app declares (from the hub), kept a minute.
type osDecl struct {
	app     bool // made from an app (a template): it can declare
	os, net []string
	at      time.Time
	// declares: the hub says os is all the app declares (hubs from v0.22.9)
	declares bool
	// official: one of myco's own apps (the hub says so)
	official bool
}

// strict: what the app does not declare is refused, not asked. Pages with
// no app have nothing to declare in, and older hubs do not say; for a
// while, myco's own apps that declare nothing yet are asked as before.
func (d osDecl) strict() bool {
	return d.app && d.declares && !(d.official && len(d.os) == 0)
}

func osKey(hub, page string) string { return hub + "\x00" + page }

func (b *Backend) grants() *osGrants {
	b.os.grantsOnce.Do(func() {
		g := &osGrants{pages: map[string]*osPerm{}, decl: map[string]osDecl{}}
		if b.Store != nil && b.Store.path != "" {
			g.file = filepath.Join(filepath.Dir(b.Store.path), "app-grants.json")
			g.load()
		}
		b.os.grants = g
	})
	return b.os.grants
}

func (g *osGrants) perm(hub, page string) *osPerm {
	k := osKey(hub, page)
	p := g.pages[k]
	if p == nil {
		p = &osPerm{once: map[string]bool{}, always: map[string]bool{}, denied: map[string]bool{}, hosts: map[string]bool{}, onceHosts: map[string]bool{}}
		g.pages[k] = p
	}
	return p
}

func (g *osGrants) load() {
	raw, err := os.ReadFile(g.file)
	if err != nil {
		return
	}
	var list []osGrant
	if json.Unmarshal(raw, &list) != nil {
		return
	}
	for _, e := range list {
		p := g.perm(e.Hub, e.Page)
		p.name = e.Name
		for _, gr := range e.Groups {
			if !osOnceOnly[gr] {
				p.always[gr] = true
			}
		}
		for _, h := range e.Hosts {
			p.hosts[h] = true
		}
	}
}

// saveLocked writes what is allowed always (g.mu held).
func (g *osGrants) saveLocked() {
	if g.file == "" {
		return
	}
	var list []osGrant
	for k, p := range g.pages {
		hub, page, _ := strings.Cut(k, "\x00")
		e := osGrant{Hub: hub, Page: page, Name: p.name, At: time.Now().Unix()}
		for gr := range p.always {
			e.Groups = append(e.Groups, gr)
		}
		for h := range p.hosts {
			e.Hosts = append(e.Hosts, h)
		}
		if len(e.Groups) == 0 && len(e.Hosts) == 0 {
			continue
		}
		sort.Strings(e.Groups)
		sort.Strings(e.Hosts)
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Hub+list[i].Page < list[j].Hub+list[j].Page })
	raw, _ := json.MarshalIndent(list, "", "  ")
	if os.MkdirAll(filepath.Dir(g.file), 0o700) == nil {
		_ = os.WriteFile(g.file, append(raw, '\n'), 0o600)
	}
}

// declared is what the page's app declares (asked of its hub; a minute's cache).
func (b *Backend) declared(ctx context.Context, hub, page string) osDecl {
	g := b.grants()
	k := osKey(hub, page)
	g.mu.Lock()
	d, ok := g.decl[k]
	g.mu.Unlock()
	if ok && time.Since(d.at) < time.Minute {
		return d
	}
	d = osDecl{at: time.Now()}
	if b.Store == nil {
		return d // (no hubs: nothing declared; the person is asked as for any page)
	}
	if h, err := b.find(hub); err == nil {
		if c, err := b.client(h); err == nil {
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			p, err := c.Page(cctx, page)
			cancel()
			if err == nil && p != nil {
				d = declOf(p)
			}
		}
	}
	g.mu.Lock()
	g.decl[k] = d
	g.mu.Unlock()
	return d
}

func declOf(p *api.Page) osDecl {
	d := osDecl{at: time.Now()}
	if p.Template != nil {
		d.app = true
		d.os, d.net = p.Template.OS, p.Template.Net
		d.declares, d.official = p.Template.Declares, p.Template.Official
	}
	return d
}

// osConsent is the answer to a call the person has not allowed yet.
type osConsent struct {
	Error    string `json:"error"` // consent_required
	Message  string `json:"message"`
	Group    string `json:"group"`
	Host     string `json:"host,omitempty"`
	Declared bool   `json:"declared"`  // the app says it uses this group (or has no manifest to say it in)
	OnceOnly bool   `json:"once_only"` // may only be allowed for this run
}

// osCheck says whether a page may make a call now: nil, or why not (a
// consent to ask for, or a refusal).
func (b *Backend) osCheck(ctx context.Context, in osCallIn) (*osConsent, error) {
	group := osGroupOf(in.Name)
	if group == "" || in.Page.Hub == "" || in.Page.ID == "" {
		// (no page: the App itself, or a tool of its own)
		return nil, nil
	}
	g := b.grants()
	g.mu.Lock()
	p := g.perm(in.Page.Hub, in.Page.ID)
	if in.Page.Name != "" {
		p.name = in.Page.Name
	}
	picked := append([]string(nil), p.picked...)
	denied, allowed := p.denied[group], p.once[group] || p.always[group]
	g.mu.Unlock()
	if pickFree[in.Name] || (group == "files" || group == "files.write" || in.Name == "fs.open") && within(osPaths(in.Name, in.Args), picked) {
		// (the person's own pick: still only for an app that says it uses files)
		if d := b.declared(ctx, in.Page.Hub, in.Page.ID); d.strict() && !hasString(d.os, group) {
			return nil, undeclaredOS(group)
		}
		return nil, nil
	}
	d := b.declared(ctx, in.Page.Hub, in.Page.ID)
	declared := !d.app || hasString(d.os, group)
	if !declared && d.strict() {
		return nil, undeclaredOS(group)
	}
	if group == "network" {
		host := fetchHost(in.Args)
		if host == "" {
			return nil, nil // (oscap refuses the call itself)
		}
		g.mu.Lock()
		hostOK := p.hosts[host] || p.onceHosts[host]
		g.mu.Unlock()
		switch {
		case hostMatches(d.net, host):
			// declared hosts: the group's consent is enough
		case isPrivateHost(host) && d.app && len(d.net) > 0:
			return nil, &osRefusal{Code: "permission_denied", Message: "this app may not reach " + host + ": a private address it does not declare in myco.json's \"net\""}
		case !hostOK:
			if denied {
				return nil, &osRefusal{Code: "permission_denied", Message: "the person did not allow this app to use the network"}
			}
			return &osConsent{Error: "consent_required", Message: "ask the person first", Group: group, Host: host, Declared: declared && len(d.net) == 0, OnceOnly: false}, nil
		}
	}
	if denied {
		return nil, &osRefusal{Code: "permission_denied", Message: "the person did not allow this app: " + group}
	}
	if allowed {
		return nil, nil
	}
	return &osConsent{Error: "consent_required", Message: "ask the person first", Group: group, Declared: declared, OnceOnly: osOnceOnly[group]}, nil
}

// undeclaredOS: an app may not use a group of the computer's capabilities
// it does not declare.
func undeclaredOS(group string) *osRefusal {
	return &osRefusal{Code: "permission_denied", Message: fmt.Sprintf("this app does not declare %q in myco.json's \"os\": add it there to use it", group)}
}

type osRefusal struct{ Code, Message string }

func (e *osRefusal) Error() string { return e.Code + ": " + e.Message }

// osPicked keeps what the person picked for the page (fs.pick, fs.saveAs answers).
func (b *Backend) osPicked(in osCallIn, value any) {
	if !pickFree[in.Name] || in.Page.ID == "" {
		return
	}
	var paths []string
	switch v := value.(type) {
	case map[string]any:
		if s, ok := v["path"].(string); ok {
			paths = append(paths, s)
		}
		if l, ok := v["paths"].([]any); ok {
			for _, x := range l {
				if s, ok := x.(string); ok {
					paths = append(paths, s)
				}
			}
		}
	default:
		raw, _ := json.Marshal(value)
		var r struct {
			Path  string   `json:"path"`
			Paths []string `json:"paths"`
		}
		if json.Unmarshal(raw, &r) == nil {
			if r.Path != "" {
				paths = append(paths, r.Path)
			}
			paths = append(paths, r.Paths...)
		}
	}
	if len(paths) == 0 {
		return
	}
	g := b.grants()
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.perm(in.Page.Hub, in.Page.ID)
	for _, s := range paths {
		if s = filepath.Clean(s); s != "." && !hasString(p.picked, s) {
			p.picked = append(p.picked, s)
		}
	}
	if len(p.picked) > 500 {
		p.picked = p.picked[len(p.picked)-500:]
	}
}

// osPaths are the paths a files call names.
func osPaths(name string, args json.RawMessage) []string {
	var a struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
		From  string   `json:"from"`
		To    string   `json:"to"`
	}
	_ = json.Unmarshal(args, &a)
	var out []string
	for _, s := range append([]string{a.Path, a.From, a.To}, a.Paths...) {
		if s != "" {
			out = append(out, s)
		}
	}
	if name == "fs.paths" {
		return nil
	}
	return out
}

// within: every path is a picked one, or inside a picked folder.
func within(paths, picked []string) bool {
	if len(paths) == 0 || len(picked) == 0 {
		return false
	}
	for _, s := range paths {
		if strings.HasPrefix(s, "~") {
			return false
		}
		c := filepath.Clean(s)
		ok := false
		for _, p := range picked {
			if c == p || strings.HasPrefix(c, p+string(filepath.Separator)) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func fetchHost(args json.RawMessage) string {
	var a struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(args, &a) != nil {
		return ""
	}
	u, err := url.Parse(a.URL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

// hostMatches: host (with its port, if any) is one the manifest lists:
// exactly, without a port when the list has none, or under a *.domain.
func hostMatches(list []string, host string) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	for _, h := range list {
		switch {
		case h == host, h == name:
			return true
		case strings.HasPrefix(h, "*."):
			if strings.HasSuffix(name, h[1:]) {
				return true
			}
		}
	}
	return false
}

// isPrivateHost: this computer, the local network, or a name only it knows.
func isPrivateHost(host string) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	name = strings.Trim(name, "[]")
	if name == "localhost" || strings.HasSuffix(name, ".localhost") || strings.HasSuffix(name, ".local") || !strings.Contains(name, ".") {
		return true
	}
	if ip := net.ParseIP(name); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
	}
	return false
}

func hasString(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func (b *Backend) grantRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/os/grants", func(w http.ResponseWriter, r *http.Request) {
		g := b.grants()
		g.mu.Lock()
		type row struct {
			Hub    string   `json:"hub"`
			Page   string   `json:"page"`
			Name   string   `json:"name,omitempty"`
			Always []string `json:"always,omitempty"`
			Once   []string `json:"once,omitempty"`
			Denied []string `json:"denied,omitempty"`
			Hosts  []string `json:"hosts,omitempty"`
		}
		keys := func(m map[string]bool) []string {
			var out []string
			for k := range m {
				out = append(out, k)
			}
			sort.Strings(out)
			return out
		}
		out := []row{}
		for k, p := range g.pages {
			hub, page, _ := strings.Cut(k, "\x00")
			e := row{Hub: hub, Page: page, Name: p.name, Always: keys(p.always), Once: keys(p.once), Denied: keys(p.denied), Hosts: keys(p.hosts)}
			if len(e.Always)+len(e.Once)+len(e.Denied)+len(e.Hosts) > 0 {
				out = append(out, e)
			}
		}
		g.mu.Unlock()
		sort.Slice(out, func(i, j int) bool { return out[i].Name+out[i].Page < out[j].Name+out[j].Page })
		writeJSON(w, http.StatusOK, map[string]any{"grants": out})
	})
	m.HandleFunc("POST /app/os/grants", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Hub, Page, Name, Group, Host, Answer string
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Hub == "" || in.Page == "" || in.Group == "" || (in.Answer != "once" && in.Answer != "always" && in.Answer != "deny") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_argument", "message": "hub, page, group and answer (once, always or deny)"})
			return
		}
		g := b.grants()
		g.mu.Lock()
		defer g.mu.Unlock()
		p := g.perm(in.Hub, in.Page)
		if in.Name != "" {
			p.name = in.Name
		}
		answer := in.Answer
		if answer == "always" && osOnceOnly[in.Group] {
			answer = "once"
		}
		switch {
		case answer == "deny":
			p.denied[in.Group] = true
			delete(p.once, in.Group)
		case in.Group == "network" && in.Host != "":
			// a host beyond those the app declares; the network itself too
			if answer == "always" {
				p.hosts[in.Host] = true
				p.always[in.Group] = true
			} else {
				p.onceHosts[in.Host] = true
				p.once[in.Group] = true
			}
			delete(p.denied, in.Group)
		case answer == "always":
			p.always[in.Group] = true
			delete(p.denied, in.Group)
		default:
			p.once[in.Group] = true
			delete(p.denied, in.Group)
		}
		if answer == "always" {
			g.saveLocked()
		}
		if answer == "deny" && in.Group == "browser" {
			b.stopBrowserPage(in.Hub, in.Page)
		}
		writeJSON(w, http.StatusOK, map[string]any{"group": in.Group, "answer": answer})
	})
	m.HandleFunc("DELETE /app/os/grants", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		hub, page, group := q.Get("hub"), q.Get("page"), q.Get("group")
		g := b.grants()
		g.mu.Lock()
		defer g.mu.Unlock()
		if p := g.pages[osKey(hub, page)]; p != nil {
			if group == "" {
				delete(g.pages, osKey(hub, page))
			} else {
				delete(p.always, group)
				delete(p.once, group)
				delete(p.denied, group)
				if group == "network" {
					p.hosts, p.onceHosts = map[string]bool{}, map[string]bool{}
				}
			}
			g.saveLocked()
		}
		if group == "" || group == "browser" {
			b.stopBrowserPage(hub, page)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
