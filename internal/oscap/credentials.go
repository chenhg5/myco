package oscap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/myco-ai/myco/internal/api"
)

func init() {
	register(&Cap{Name: "credentials.http", About: "Read-only HTTP requests using opaque local credentials (v1)", Fn: func(context.Context, *Env, json.RawMessage, Emit) (any, error) {
		return map[string]int{"version": 1}, nil
	}}, &Cap{Name: "credentials.connections", About: "Manage page-scoped credentials without exposing secrets", Fn: credentialConnections})
}

var credentialMu sync.Mutex

type savedCredential struct {
	Connection   api.CredentialConnection `json:"connection"`
	Token        string                   `json:"token"`
	AllowPrivate bool                     `json:"allow_private"`
	BasePath     string                   `json:"base_path"`
}

func credentialKey(e *Env) (string, error) {
	if e == nil || e.Home == "" || len(strings.Split(e.Scope, "\x00")) != 5 || e.Secrets == nil {
		return "", errf(CodePermission, "a signed-in workspace, application and page are required")
	}
	h := sha256.Sum256([]byte(e.Scope))
	return "app-credential-" + hex.EncodeToString(h[:]), nil
}
func credentialFile(e *Env, key string) string {
	return filepath.Join(e.Home, ".myco", "apps", "credentials", key+".json")
}
func credentialLoad(e *Env, key string) ([]string, error) {
	raw, err := os.ReadFile(credentialFile(e, key))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, errf(CodeFailed, "could not read connections")
	}
	var ids []string
	if json.Unmarshal(raw, &ids) != nil {
		return nil, errf(CodeFailed, "connection settings are damaged")
	}
	return ids, nil
}
func credentialSave(e *Env, key string, ids []string) error {
	path := credentialFile(e, key)
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return errf(CodeFailed, "could not save connections")
	}
	raw, _ := json.Marshal(ids)
	f, err := os.CreateTemp(filepath.Dir(path), "connections-*.tmp")
	if err != nil {
		return errf(CodeFailed, "could not save connections")
	}
	defer os.Remove(f.Name())
	err = f.Chmod(0600)
	if err == nil {
		_, err = f.Write(raw)
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return errf(CodeFailed, "could not save connections")
	}
	return nil
}
func credentialRead(e *Env, key, id string) (savedCredential, error) {
	var s savedCredential
	if !strings.HasPrefix(id, "c_") || len(id) != 26 {
		return s, errf(CodeNotFound, "connection not found")
	}
	raw, err := e.Secrets.Get(key + "-" + id)
	if err != nil || raw == "" {
		return s, errf(CodeNoCredentials, "reconnect this connection")
	}
	if json.Unmarshal([]byte(raw), &s) != nil || s.Connection.ID != id {
		return s, errf(CodePermission, "invalid saved connection")
	}
	return s, nil
}
func credentialLookup(e *Env, id string) (savedCredential, error) {
	key, err := credentialKey(e)
	if err != nil {
		return savedCredential{}, err
	}
	credentialMu.Lock()
	defer credentialMu.Unlock()
	ids, err := credentialLoad(e, key)
	if err != nil {
		return savedCredential{}, err
	}
	for _, v := range ids {
		if v == id {
			return credentialRead(e, key, id)
		}
	}
	return savedCredential{}, errf(CodeNotFound, "connection not found in this page")
}
func credentialConnections(_ context.Context, e *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Action string `json:"action"`
		ID     string `json:"id"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	key, err := credentialKey(e)
	if err != nil {
		return nil, err
	}
	credentialMu.Lock()
	defer credentialMu.Unlock()
	ids, err := credentialLoad(e, key)
	if err != nil {
		return nil, err
	}
	if in.Action == "list" {
		out := []api.CredentialConnection{}
		for _, id := range ids {
			s, err := credentialRead(e, key, id)
			if err != nil {
				return nil, err
			}
			out = append(out, s.Connection)
		}
		return out, nil
	}
	if in.Action != "remove" {
		return nil, errf(CodePermission, "new connections require the host dialog")
	}
	for i, id := range ids {
		if id == in.ID {
			if err := e.Secrets.Delete(key + "-" + id); err != nil {
				return nil, errf(CodeFailed, "could not remove credential")
			}
			return true, credentialSave(e, key, append(ids[:i], ids[i+1:]...))
		}
	}
	return nil, errf(CodeNotFound, "connection not found")
}

// CredentialOrigin validates the exact HTTPS destination shown by the host.
func CredentialOrigin(origin string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(u.Host, "\\\r\n\x00%") || strings.HasSuffix(u.Hostname(), ".") {
		return "", errf(CodeInvalid, "enter an HTTPS origin without a path or credentials")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return "", errf(CodeInvalid, "invalid server port")
		}
		if _, err := url.ParseRequestURI("https://" + u.Host); err != nil {
			return "", errf(CodeInvalid, "invalid server port")
		}
	}
	host := strings.ToLower(u.Host)
	if u.Port() == "443" {
		host = strings.TrimSuffix(host, ":443")
	}
	return "https://" + host, nil
}
func credentialPath(path string) error {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\\?#\r\n\x00") || strings.Contains(path, "//") {
		return errf(CodeInvalid, "invalid request path")
	}
	// Decode once only; reject nested escapes rather than interpreting them twice.
	lower := strings.ToLower(path)
	for _, v := range []string{"%2e", "%2f", "%5c", "%25"} {
		if strings.Contains(lower, v) {
			return errf(CodeInvalid, "encoded path structure is not allowed")
		}
	}
	decoded, err := url.PathUnescape(path)
	if err != nil || strings.ContainsAny(decoded, "?#\\") {
		return errf(CodeInvalid, "invalid path encoding")
	}
	for _, r := range decoded {
		if r < 32 || r == 127 {
			return errf(CodeInvalid, "invalid path character")
		}
	}
	for _, p := range strings.Split(decoded[1:], "/") {
		if p == "" || p == "." || p == ".." {
			return errf(CodeInvalid, "empty or dot path segments are not allowed")
		}
	}
	return nil
}
func CredentialDescriptorValid(d api.CredentialDescriptor) (api.CredentialDescriptor, error) {
	origin, err := CredentialOrigin(d.Origin)
	if err != nil {
		return d, err
	}
	d.Origin = origin
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || len(d.Name) > 100 || len(d.Paths) == 0 || len(d.Paths) > 32 {
		return d, errf(CodeInvalid, "name and specific allowed paths are required")
	}
	switch d.Authentication {
	case "bearer", "token", "private-token":
	default:
		return d, errf(CodeInvalid, "unsupported authentication scheme")
	}
	for _, p := range d.Paths {
		path := p.Path
		if p.Prefix {
			if !strings.HasSuffix(path, "/") {
				return d, errf(CodeInvalid, "path prefixes must end with a slash")
			}
			path = strings.TrimSuffix(path, "/")
		}
		if path == "" || credentialPath(path) != nil || strings.Contains(path, "%") {
			return d, errf(CodeInvalid, "declare literal, specific path rules")
		}
	}
	return d, nil
}

// CredentialStore is called only after trusted-host confirmation; never registered as a capability.
func CredentialStore(e *Env, d api.CredentialDescriptor, token, source, base string, private bool) (api.CredentialConnection, error) {
	d, err := CredentialDescriptorValid(d)
	if err != nil {
		return api.CredentialConnection{}, err
	}
	key, err := credentialKey(e)
	if err != nil {
		return api.CredentialConnection{}, err
	}
	if len(token) < 8 || len(token) > 8192 || strings.ContainsAny(token, "\r\n\x00") {
		return api.CredentialConnection{}, errf(CodeInvalid, "enter a valid credential")
	}
	credentialMu.Lock()
	defer credentialMu.Unlock()
	ids, err := credentialLoad(e, key)
	if err != nil {
		return api.CredentialConnection{}, err
	}
	if len(ids) >= 24 {
		return api.CredentialConnection{}, errf(CodeTooLarge, "at most 24 connections per page")
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return api.CredentialConnection{}, errf(CodeFailed, "could not create connection")
	}
	c := api.CredentialConnection{ID: "c_" + hex.EncodeToString(random[:]), CredentialDescriptor: d, Source: source}
	raw, _ := json.Marshal(savedCredential{Connection: c, Token: token, AllowPrivate: private, BasePath: base})
	if e.Secrets.Set(key+"-"+c.ID, string(raw)) != nil {
		return c, errf(CodeFailed, "could not save credential")
	}
	if err = credentialSave(e, key, append(ids, c.ID)); err != nil {
		_ = e.Secrets.Delete(key + "-" + c.ID)
		return api.CredentialConnection{}, err
	}
	return c, nil
}

// CredentialRequestURL resolves the destination for the existing network grant checker.
func CredentialRequestURL(e *Env, id, path string) (string, error) {
	s, err := credentialLookup(e, id)
	if err != nil {
		return "", err
	}
	if err = credentialPath(path); err != nil {
		return "", err
	}
	decoded, _ := url.PathUnescape(path)
	if s.BasePath != "" && !strings.HasPrefix(decoded, s.BasePath+"/") {
		return "", errf(CodePermission, "path is outside the credential source API")
	}
	for _, p := range s.Connection.Paths {
		if (!p.Prefix && decoded == p.Path) || (p.Prefix && strings.HasPrefix(decoded, p.Path)) {
			return s.Connection.Origin + path, nil
		}
	}
	return "", errf(CodePermission, "path is outside this connection's grant")
}
