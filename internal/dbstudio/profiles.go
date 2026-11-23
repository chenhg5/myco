package dbstudio

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var profileMu sync.Mutex
var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type Store struct {
	Home, Scope string
	Secrets     Secrets
}

func (s Store) key() string {
	h := sha256.Sum256([]byte(s.Scope))
	return "app-database-" + hex.EncodeToString(h[:16])
}
func (s Store) credentialKey(p Profile) string {
	return s.key() + "-" + p.ID + "-v" + strconv.Itoa(p.Version)
}
func (s Store) file() string {
	return filepath.Join(s.Home, ".myco", "apps", "database", s.key()+".json")
}
func (s Store) load() ([]Profile, error) {
	b, err := os.ReadFile(s.file())
	if os.IsNotExist(err) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, failure("failed", "could not read saved connections")
	}
	var out []Profile
	if json.Unmarshal(b, &out) != nil {
		return nil, failure("failed", "saved connections are damaged; restore the local profile file")
	}
	return out, nil
}
func (s Store) write(rows []Profile) error {
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.file())
	if err = os.MkdirAll(dir, 0700); err != nil {
		return failure("failed", "could not save connections")
	}
	f, err := os.CreateTemp(dir, "profiles-*.tmp")
	if err != nil {
		return failure("failed", "could not save connections")
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, s.file())
	}
	if err != nil {
		return failure("failed", "could not save connections")
	}
	return nil
}
func (s Store) List() ([]Profile, error) { profileMu.Lock(); defer profileMu.Unlock(); return s.load() }
func (s Store) connection(p Profile) (Connection, error) {
	c := Connection{Profile: p}
	if !p.HasPassword && !p.HasURI {
		return c, nil
	}
	if s.Secrets == nil {
		return c, failure("unsupported", "the desktop credential store is unavailable")
	}
	raw, err := s.Secrets.Get(s.credentialKey(p))
	if err != nil || json.Unmarshal([]byte(raw), &c.secret) != nil {
		return c, failure("permission_denied", "saved credentials are unavailable; enter them again")
	}
	return c, nil
}
func (s Store) Get(id string) (Connection, error) {
	profileMu.Lock()
	defer profileMu.Unlock()
	rows, err := s.load()
	if err != nil {
		return Connection{}, err
	}
	for _, p := range rows {
		if p.ID == id {
			return s.connection(p)
		}
	}
	return Connection{}, failure("not_found", "connection not found on this computer")
}
func endpoint(p Profile) string {
	return strings.Join([]string{p.Engine, p.Host, p.User, p.Path}, "\x00") + string(rune(p.Port))
}
func prepare(s Store, in ProfileInput, old *Connection) (Connection, error) {
	p := in.Profile
	p.Name = strings.TrimSpace(p.Name)
	p.Host = strings.TrimSpace(p.Host)
	p.User = strings.TrimSpace(p.User)
	if p.Name == "" || len(p.Name) > 120 || !validID.MatchString(p.ID) {
		return Connection{}, failure("invalid_argument", "a connection name and valid ID are required")
	}
	if p.Engine != "mysql" && p.Engine != "postgres" && p.Engine != "sqlite" && p.Engine != "mongodb" {
		return Connection{}, failure("unsupported", "choose MySQL, PostgreSQL, SQLite or MongoDB")
	}
	if p.TLS == "" {
		p.TLS = "verify"
	}
	if p.TLS != "verify" && p.TLS != "require" && p.TLS != "disable" {
		return Connection{}, failure("invalid_argument", "invalid TLS mode")
	}
	if len(p.Database) > 256 || len(p.User) > 256 || len(p.AuthSource) > 256 {
		return Connection{}, failure("invalid_argument", "connection field is too long")
	}
	c := Connection{Profile: p}
	if old != nil && endpoint(old.Profile) == endpoint(p) {
		c.secret = old.secret
	}
	if in.Password != nil {
		c.secret.Password = *in.Password
	}
	if in.URI != nil {
		c.secret.URI = *in.URI
	}
	if len(c.secret.Password) > 1024 || len(c.secret.URI) > 1024 {
		return c, failure("too_large", "credential or URI is too long (maximum 1024 bytes each)")
	}
	if p.Engine == "sqlite" {
		path := p.Path
		if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
			path = filepath.Join(s.Home, path[2:])
		}
		if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
			return c, failure("invalid_argument", "choose an absolute SQLite file path")
		}
		c.Path = filepath.Clean(path)
		c.Host = ""
		c.User = ""
		c.Port = 0
		c.secret = credentials{}
	} else if c.secret.URI != "" && p.Engine == "mongodb" {
		u, err := url.Parse(c.secret.URI)
		if err != nil || (u.Scheme != "mongodb" && u.Scheme != "mongodb+srv") || u.Host == "" {
			return c, failure("invalid_argument", "enter a MongoDB connection URI")
		}
		c.Host = u.Host
		c.Port = 0
		c.User = ""
		if c.Database == "" {
			c.Database = strings.TrimPrefix(u.Path, "/")
		}
	} else {
		if p.Host == "" || len(p.Host) > 256 || strings.ContainsAny(p.Host, "/\\@?#\x00\r\n") || p.Port < 1 || p.Port > 65535 {
			return c, failure("invalid_argument", "enter a hostname and port from 1 to 65535")
		}
		if strings.Contains(p.Host, ":") && net.ParseIP(p.Host) == nil {
			return c, failure("invalid_argument", "enter the host and port separately")
		}
		c.secret.URI = ""
	}
	c.HasPassword = c.secret.Password != ""
	c.HasURI = c.secret.URI != ""
	return c, nil
}
func (s Store) Prepare(in ProfileInput) (Connection, error) {
	var old *Connection
	if in.ID != "" {
		c, err := s.Get(in.ID)
		if err == nil {
			old = &c
		}
	}
	return prepare(s, in, old)
}
func (s Store) Save(in ProfileInput) (Profile, error) {
	profileMu.Lock()
	defer profileMu.Unlock()
	rows, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	if in.ID == "" {
		b := make([]byte, 12)
		if _, err = rand.Read(b); err != nil {
			return Profile{}, err
		}
		in.ID = "db_" + hex.EncodeToString(b)
	}
	var old *Connection
	index := -1
	for i, p := range rows {
		if p.ID == in.ID {
			v, x := s.connection(p)
			if x != nil && (in.Password == nil && in.URI == nil) {
				return Profile{}, x
			}
			old = &v
			index = i
			break
		}
	}
	version := 0
	if old != nil {
		version = old.Version
	}
	if in.Version != version {
		return Profile{}, failure("conflict", "connection changed in another window; reload it before saving")
	}
	c, err := prepare(s, in, old)
	if err != nil {
		return Profile{}, err
	}
	c.Version = version + 1
	if index < 0 && len(rows) >= 100 {
		return Profile{}, failure("too_large", "this page already has 100 saved connections")
	}
	if c.HasPassword || c.HasURI {
		if s.Secrets == nil {
			return Profile{}, failure("unsupported", "the desktop credential store is unavailable")
		}
		b, _ := json.Marshal(c.secret)
		if err = s.Secrets.Set(s.credentialKey(c.Profile), string(b)); err != nil {
			return Profile{}, failure("permission_denied", "could not save credentials in the system credential store")
		}
	}
	if index < 0 {
		rows = append(rows, c.Profile)
	} else {
		rows[index] = c.Profile
	}
	if err = s.write(rows); err != nil {
		if s.Secrets != nil {
			_ = s.Secrets.Delete(s.credentialKey(c.Profile))
		}
		return Profile{}, err
	}
	if old != nil && s.Secrets != nil {
		_ = s.Secrets.Delete(s.credentialKey(old.Profile))
	}
	return c.Profile, nil
}
func (s Store) Forget(id string, version int) error {
	profileMu.Lock()
	defer profileMu.Unlock()
	rows, err := s.load()
	if err != nil {
		return err
	}
	for i, p := range rows {
		if p.ID == id {
			if p.Version != version {
				return failure("conflict", "connection changed in another window; reload it first")
			}
			if s.Secrets != nil {
				if err := s.Secrets.Delete(s.credentialKey(p)); err != nil {
					return failure("permission_denied", "could not erase saved credentials; unlock the credential store and try again")
				}
			}
			if err = s.write(append(rows[:i], rows[i+1:]...)); err != nil {
				return err
			}
			return nil
		}
	}
	return failure("not_found", "connection not found")
}
