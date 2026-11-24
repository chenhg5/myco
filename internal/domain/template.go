package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Templates (templates.md): ready-made pages people and agents fill with
// data. A template is a page's files plus myco.json, its manifest; using one
// makes a page (an instance) with its files and the template's data
// structure. A workspace keeps its templates in its library.

// Template is one template in a workspace's library.
type Template struct {
	ID string
	// Slug is the manifest's id (author/name), unique in the workspace.
	Slug        string
	Name        string
	Description string
	Kind        string
	// Icon is its icon (SVG markup), when it has one.
	Icon string
	// Latest is the newest version (semver).
	Latest    string
	Publisher string
	// Owner is the member id of the person it belongs to (the publishing
	// agent's owner, or the person).
	Owner string
	// Official: one of myco's, offered by the hub itself (not in the
	// library's tables; it cannot be deleted).
	Official  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TemplateVersion is one publish of a template.
type TemplateVersion struct {
	Template string
	Version  string
	Blob     string
	SHA256   string
	Size     int64
	Files    int
	// Manifest is myco.json as published (JSON).
	Manifest string
	// Guide is the agent's guide (agent.md), when it has one.
	Guide string
	// Icon is its icon (SVG markup), when it has one.
	Icon string
	By   string
	At   time.Time
}

// TemplateRepository stores the library.
type TemplateRepository interface {
	Create(ctx context.Context, t *Template) error
	Save(ctx context.Context, t *Template) error
	Get(ctx context.Context, id string) (*Template, error)
	BySlug(ctx context.Context, slug string) (*Template, error)
	List(ctx context.Context) ([]*Template, error)
	Delete(ctx context.Context, id string) error
	AddVersion(ctx context.Context, v *TemplateVersion) error
	// Versions lists a template's versions, newest first.
	Versions(ctx context.Context, template string) ([]*TemplateVersion, error)
	DeleteVersion(ctx context.Context, template, version string) error
}

const (
	// MaxTemplates bounds a workspace's library.
	MaxTemplates = 200
	// TemplateVersionsKept: older versions are let go.
	TemplateVersionsKept = 10
	// MaxTemplateGuide bounds agent.md.
	MaxTemplateGuide = 32 << 10
)

// Manifest is a template's myco.json.
type Manifest struct {
	ID          string                        `json:"id"`
	Name        string                        `json:"name"`
	Version     string                        `json:"version"`
	Kind        string                        `json:"kind,omitempty"`
	Category    string                        `json:"category,omitempty"`
	Description string                        `json:"description,omitempty"`
	Entry       string                        `json:"entry,omitempty"`
	Data        map[string]ManifestCollection `json:"data,omitempty"`
	Permissions []string                      `json:"permissions,omitempty"`
	// OS lists the groups of the computer's capabilities (myco.os) the app
	// uses (hubs from v0.22.1; see TemplateOSGroups), so the App can ask the
	// person for them, and refuse the rest.
	OS []string `json:"os,omitempty"`
	// Net lists the hosts its network requests may reach (hubs from
	// v0.22.1): host, host:port, or *.domain for its subdomains.
	Net    []string `json:"net,omitempty"`
	Export []string `json:"export,omitempty"`
	Agent  string   `json:"agent,omitempty"`
	Sample string   `json:"sample,omitempty"`
	// Icon is an SVG in the template (its shape is shown, tinted by the
	// App; v0.20.7).
	Icon string `json:"icon,omitempty"`
	// Requires says what the template needs (v0.20.7): hubs older than
	// Requires.Hub leave it out.
	Requires *ManifestRequires `json:"requires,omitempty"`
}

// ManifestRequires is what a template needs to run.
type ManifestRequires struct {
	Hub string `json:"hub,omitempty"`
}

// MaxTemplateIcon bounds a template's icon (an SVG).
const MaxTemplateIcon = 16 << 10

// RequiredHub reads the hub version a manifest asks for ("" when none),
// without checking anything else: a hub reads it before the rest, so it
// can leave out templates made for newer hubs (whose manifests it may not
// understand).
func RequiredHub(raw []byte) string {
	var m struct {
		Requires struct {
			Hub string `json:"hub"`
		} `json:"requires"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(m.Requires.Hub), "v")
}

// ManifestCollection is a collection of a template's data: its fields and
// their types. Single: one record (e.g. a document's body).
type ManifestCollection struct {
	Fields map[string]string `json:"fields"`
	Single bool              `json:"single,omitempty"`
	// Write "managers": only those who manage the page change this
	// collection (hubs from v0.22.2; its settings, say), whoever else may.
	Write string `json:"write,omitempty"`
}

// FieldTypes are the types a field may have.
var FieldTypes = []string{"text", "markdown", "number", "bool", "date", "datetime", "person", "select", "multi", "url", "file", "json"}

// TemplateCategories are the shelves of the library (apps, in the App):
// office suite, development, tools, life, creative, games; others are
// "other".
var TemplateCategories = []string{"office", "dev", "tools", "life", "creative", "game", "other"}

// TemplateKinds are what a template may be (custom: anything else).
var TemplateKinds = []string{"table", "records", "doc", "slides", "board", "form", "custom"}

// TemplatePermissions are what a template may ask for.
var TemplatePermissions = []string{"send", "download"}

// TemplateOSGroups are the groups of myco.os capabilities an app declares
// (manifest "os"): files (look and read, and what the person picks), files.write,
// files.delete, exec (commands, ending processes), screen, microphone, speech,
// clipboard (reading it), notify, system, network (net.fetch, to the hosts in
// "net"), ssh, database, proxy.
var TemplateOSGroups = []string{"files", "files.write", "files.delete", "exec", "screen", "microphone", "speech", "clipboard", "notify", "system", "network", "ssh", "database", "proxy"}

// netHost is a host a manifest's "net" names: a DNS name or an IP address,
// with a port or not, or *. and a domain for its subdomains.
var netHost = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(:[0-9]{1,5})?$`)

var (
	templateSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}(/[a-z0-9][a-z0-9-]{0,39})?$`)
	semver       = regexp.MustCompile(`^(\d{1,6})\.(\d{1,6})\.(\d{1,6})$`)
	fieldName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,39}$`)
)

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ParseManifest reads and checks myco.json.
func ParseManifest(raw []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, Errf(CodeInvalidArgument, "myco.json is not a valid manifest: %v", err).
			WithSuggestion("start from the one myco template init writes")
	}
	bad := func(format string, a ...any) (*Manifest, error) {
		return nil, Errf(CodeInvalidArgument, "myco.json: "+format, a...)
	}
	if !templateSlug.MatchString(m.ID) {
		return bad("id %q: use author/name in lowercase, e.g. glenn/weekly-report", m.ID)
	}
	name, err := ParsePageName(m.Name)
	if err != nil {
		return bad("name %q: one line, up to 80 characters", m.Name)
	}
	m.Name = name
	if !semver.MatchString(m.Version) {
		return bad("version %q: use major.minor.patch, e.g. 1.0.0", m.Version)
	}
	if m.Kind == "" {
		m.Kind = "custom"
	}
	if !has(TemplateKinds, m.Kind) {
		return bad("kind %q: one of %s", m.Kind, strings.Join(TemplateKinds, ", "))
	}
	if m.Category != "" && !has(TemplateCategories, m.Category) {
		return bad("category %q: one of %s", m.Category, strings.Join(TemplateCategories, ", "))
	}
	if utf8.RuneCountInString(m.Description) > 300 {
		return bad("description: 300 characters at most")
	}
	if m.Entry == "" {
		m.Entry = "index.html"
	}
	if m.Entry, err = CleanPagePath(m.Entry); err != nil {
		return bad("entry %q is not a path in the template", m.Entry)
	}
	if len(m.Data) > 20 {
		return bad("data: 20 collections at most")
	}
	for c, col := range m.Data {
		if _, err := ParseCollection(c); err != nil {
			return bad("data: invalid collection %q (lowercase letters, digits, - and _)", c)
		}
		if len(col.Fields) == 0 || len(col.Fields) > 60 {
			return bad("data.%s: give it 1 to 60 fields", c)
		}
		for f, t := range col.Fields {
			if !fieldName.MatchString(f) || f == "id" {
				return bad("data.%s: invalid field %q (letters, digits and _, not id)", c, f)
			}
			if !has(FieldTypes, t) {
				return bad("data.%s.%s: type %q is not one of %s", c, f, t, strings.Join(FieldTypes, ", "))
			}
		}
		if col.Write != "" && col.Write != PageEditorsManagers {
			return bad("data.%s.write %q: managers, or leave it out", c, col.Write)
		}
	}
	for _, p := range m.Permissions {
		if !has(TemplatePermissions, p) {
			return bad("permission %q: one of %s", p, strings.Join(TemplatePermissions, ", "))
		}
	}
	for _, g := range m.OS {
		if !has(TemplateOSGroups, g) {
			return bad("os %q: one of %s", g, strings.Join(TemplateOSGroups, ", "))
		}
	}
	if len(m.Net) > 50 {
		return bad("net: at most 50 hosts")
	}
	for _, h := range m.Net {
		if !netHost.MatchString(h) || len(h) > 260 {
			return bad("net %q: a host in lowercase (api.example.com), with :port or not, or *.example.com", h)
		}
	}
	if len(m.Net) > 0 && !has(m.OS, "network") {
		return bad("net lists hosts but os has no \"network\"")
	}
	if m.Icon != "" {
		if m.Icon, err = CleanPagePath(m.Icon); err != nil || !strings.HasSuffix(strings.ToLower(m.Icon), ".svg") {
			return bad("icon %q: an .svg file in the template", m.Icon)
		}
	}
	if m.Requires != nil && m.Requires.Hub != "" {
		m.Requires.Hub = strings.TrimPrefix(m.Requires.Hub, "v")
		if !semver.MatchString(m.Requires.Hub) {
			return bad("requires.hub %q: a version like 0.20.6", m.Requires.Hub)
		}
	}
	if m.Agent == "" {
		m.Agent = "agent.md"
	}
	if m.Sample == "" {
		m.Sample = "sample.json"
	}
	return &m, nil
}

// CompareVersions compares two semver versions (-1, 0, 1).
// IsVersion reports whether s is a version like 1.2.3.
func IsVersion(s string) bool { return semver.MatchString(s) }

func CompareVersions(a, b string) int {
	pa, pb := semver.FindStringSubmatch(a), semver.FindStringSubmatch(b)
	for i := 1; i <= 3; i++ {
		var x, y int
		if pa != nil {
			x, _ = strconv.Atoi(pa[i])
		}
		if pb != nil {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// CheckRecord checks a record against a collection's fields: no fields it
// does not have, each of its type (null clears one).
func (c ManifestCollection) CheckRecord(collection, data string) error {
	return c.CheckRecordKeeping(collection, data, nil)
}

// CheckRecordKeeping is CheckRecord, letting through the fields in kept
// that the collection no longer has: a record written before its page was
// upgraded to a version without them keeps them as they are.
func (c ManifestCollection) CheckRecordKeeping(collection, data string, kept map[string]json.RawMessage) error {
	var rec map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &rec); err != nil {
		return Errf(CodeInvalidArgument, "a record is a JSON object")
	}
	keys := make([]string, 0, len(rec))
	for k := range rec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t, ok := c.Fields[k]
		if !ok {
			if old, was := kept[k]; was && jsonEqual(old, rec[k]) {
				continue
			}
			names := make([]string, 0, len(c.Fields))
			for f := range c.Fields {
				names = append(names, f)
			}
			sort.Strings(names)
			return Errf(CodeInvalidArgument, "%s has no field %q", collection, k).
				WithSuggestion("its fields are %s", strings.Join(names, ", "))
		}
		if err := checkField(rec[k], t); err != nil {
			return Errf(CodeInvalidArgument, "%s.%s: %v", collection, k, err)
		}
	}
	return nil
}

func checkField(raw json.RawMessage, t string) error {
	if string(raw) == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	str := func() (string, error) {
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("should be a string (%s)", t)
		}
		return s, nil
	}
	switch t {
	case "text", "markdown", "person", "select", "url":
		_, err := str()
		return err
	case "number":
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("should be a number")
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("should be true or false")
		}
	case "date":
		s, err := str()
		if err != nil {
			return err
		}
		if _, e := time.Parse("2006-01-02", s); e != nil {
			return fmt.Errorf("should be a date like 2026-11-18")
		}
	case "datetime":
		s, err := str()
		if err != nil {
			return err
		}
		if _, e := time.Parse(time.RFC3339, s); e != nil {
			return fmt.Errorf("should be a time like 2026-11-18T09:30:00+08:00")
		}
	case "multi":
		xs, ok := v.([]any)
		if !ok {
			return fmt.Errorf("should be a list of strings")
		}
		for _, x := range xs {
			if _, ok := x.(string); !ok {
				return fmt.Errorf("should be a list of strings")
			}
		}
	case "file":
		o, ok := v.(map[string]any)
		if !ok || o["file_id"] == nil {
			return fmt.Errorf("should be a file: {file_id, name, size, content_type}")
		}
	}
	return nil
}

// jsonEqual reports whether two JSON values are the same value.
func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}
