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

// Groups that may only be allowed for this run of the App, never always:
// running commands and deleting files can do harm no later look undoes.
// (The screen and the browser may be allowed always, and taken back in
// Settings.)
var osOnceOnly = map[string]bool{"exec": true, "files.delete": true}

type osGrant struct {
	Hub    string   `json:"hub"`
	Page   string   `json:"page"`
	Name   string   `json:"name,omitempty"` // the page's name when allowed, to show
	Groups []string `json:"groups"`
	Hosts  []string `json:"hosts,omitempty"` // network: hosts allowed beyond its manifest's
	// Version: the page's code these were allowed for (osDecl.ver). A
	// newer version of an app keeps what its manifest still declares;
	// grants kept before v0.22.11 have none and are asked again.
	Version string `json:"version,omitempty"`
	// Net: the hosts its manifest declared when the network was allowed;
	// hosts a newer version adds are asked for.
	Net []string `json:"net,omitempty"`
	At  int64    `json:"at"`
}

type osPerm struct {
	once, always, denied map[string]bool
	hosts, onceHosts     map[string]bool // network: allowed always (kept), for this run
	picked               []string
	name                 string
	// ver: the page's code its allows are for; denials hold for every version
	ver string
	// net: the declared hosts the network's allow covers (nil: not known
	// yet, it covers what the manifest declares now)
	net []string
}

// forVersion moves the allows to the code the page runs now. An app's new
// version keeps the groups (and hosts) the person allowed that its manifest
// still declares: only what it adds is asked for. A page with no manifest
// (nothing says what its new code uses) is asked again, and so are
// once-only groups. Denials hold for every version. Says whether it
// changed anything (to save).
func (p *osPerm) forVersion(d osDecl) bool {
	if !d.known || p.ver == d.ver {
		return false
	}
	if p.ver == osVerPending {
		// (allowed while the hub did not answer: those answers are for this code)
		p.ver = d.ver
		return true
	}
	keep := func(m map[string]bool) {
		for gr := range m {
			if !d.app || p.ver == "" || osOnceOnly[gr] || !hasString(d.os, gr) {
				delete(m, gr)
			}
		}
	}
	keep(p.once)
	keep(p.always)
	if !p.once["network"] && !p.always["network"] {
		p.hosts, p.onceHosts, p.net = map[string]bool{}, map[string]bool{}, nil
	}
	p.ver = d.ver
	return true
}

// osVerPending marks allows answered while the page's version was unknown.
const osVerPending = "\x00pending"

// netAllowed records which declared hosts an allow of the network covers.
func (p *osPerm) netAllowed(d osDecl) {
	if p.net == nil {
		p.net = append([]string{}, d.net...)
	}
}

type osGrants struct {
	mu    sync.Mutex
	file  string
	pages map[string]*osPerm // hub \x00 page
	decl  map[string]osDecl
}

// osDecl is what a page's app declares (from the hub), kept a minute.
type osDecl struct {
	// known: the hub answered (or there is no hub to ask). An unknown
	// declaration (a slow or failed request) changes no one's allows.
	known   bool
	app     bool // made from an app (a template): it can declare
	os, net []string
	at      time.Time
	// declares: the hub says os is all the app declares (hubs from v0.22.9)
	declares bool
	// official: one of myco's own apps (the hub says so)
	official bool
	// ver: which code the page runs: its version, and its app's
	ver string
}

// strict: what the app does not declare is refused, not asked. Pages with
// no app have nothing to declare in; older hubs do not say, and hubs say
// no for a page of myco's own apps kept from before declarations (until
// it is upgraded).
func (d osDecl) strict() bool {
	return d.app && d.declares
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
		p.name, p.ver = e.Name, e.Version
		if e.Net != nil {
			p.net = e.Net
		}
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
		e := osGrant{Hub: hub, Page: page, Name: p.name, Version: p.ver, At: time.Now().Unix()}
		if p.always["network"] {
			e.Net = p.net
		}
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
	last := d
	d = osDecl{at: time.Now()}
	if b.Store == nil {
		d.known = true
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
	if !d.known {
		// A slow or failed request is not a new version of the page: keep
		// what the hub said last (asked again in a minute), or nothing known.
		if ok && last.known {
			d = last
		}
		d.at = time.Now()
	}
	g.mu.Lock()
	g.decl[k] = d
	g.mu.Unlock()
	return d
}

func declOf(p *api.Page) osDecl {
	d := osDecl{at: time.Now(), known: true}
	if p.Template != nil {
		d.app = true
		d.os, d.net = p.Template.OS, p.Template.Net
		d.declares, d.official = p.Template.Declares, p.Template.Official
	}
	if p.Version > 0 || p.Template != nil {
		d.ver = fmt.Sprintf("%d", p.Version)
		if p.Template != nil {
			d.ver += "/" + p.Template.Version
		}
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
	d := b.declared(ctx, in.Page.Hub, in.Page.ID)
	g := b.grants()
	g.mu.Lock()
	p := g.perm(in.Page.Hub, in.Page.ID)
	if in.Page.Name != "" {
		p.name = in.Page.Name
	}
	if p.forVersion(d) {
		g.saveLocked()
	}
	if (p.once["network"] || p.always["network"]) && p.net == nil {
		p.netAllowed(d) // (allowed before hosts were kept: what it declares now)
	}
	netCovered := append([]string(nil), p.net...)
	picked := append([]string(nil), p.picked...)
	denied, allowed := p.denied[group], p.once[group] || p.always[group]
	g.mu.Unlock()
	if pickFree[in.Name] || (group == "files" || group == "files.write" || in.Name == "fs.open") && within(osPaths(in.Name, in.Args), picked) {
		// (the person's own pick: still only for an app that says it uses files)
		if d.strict() && !hasString(d.os, group) {
			return nil, undeclaredOS(group)
		}
		return nil, nil
	}
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
		case hostMatches(d.net, host) && (!allowed || hostMatches(netCovered, host)):
			// declared hosts: the group's consent is enough (a host a newer
			// version adds is asked for, below)
		case !hostMatches(d.net, host) && isPrivateHost(host) && d.app && len(d.net) > 0:
			return nil, &osRefusal{Code: "permission_denied", Message: "this app may not reach " + host + ": a private address it does not declare in myco.json's \"net\""}
		case !hostOK:
			if denied {
				return nil, &osRefusal{Code: "permission_denied", Message: "the person did not allow this app to use the network"}
			}
			return &osConsent{Error: "consent_required", Message: "ask the person first", Group: group, Host: host, Declared: declared && (len(d.net) == 0 || hostMatches(d.net, host)), OnceOnly: false}, nil
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
		d := b.declared(r.Context(), in.Hub, in.Page)
		g := b.grants()
		g.mu.Lock()
		defer g.mu.Unlock()
		p := g.perm(in.Hub, in.Page)
		if in.Name != "" {
			p.name = in.Name
		}
		// (an answer is for the code the page runs now)
		p.forVersion(d)
		if !d.known && p.ver == "" {
			p.ver = osVerPending
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
			p.netAllowed(d)
			delete(p.denied, in.Group)
		case answer == "always":
			p.always[in.Group] = true
			delete(p.denied, in.Group)
		default:
			p.once[in.Group] = true
			delete(p.denied, in.Group)
		}
		if in.Group == "network" && answer != "deny" {
			p.netAllowed(d)
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
					p.hosts, p.onceHosts, p.net = map[string]bool{}, map[string]bool{}, nil
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
