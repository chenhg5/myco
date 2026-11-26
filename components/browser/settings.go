package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type proxySettings struct {
	Server      string   `json:"server"`
	Bypass      []string `json:"bypass"`
	Username    string   `json:"username"`
	HasPassword bool     `json:"has_password"`
}
type profileSettings struct {
	Proxy     *proxySettings `json:"proxy"`
	UserAgent string         `json:"user_agent"`
	Lang      string         `json:"lang"`
	Args      []string       `json:"args"`
}
type proxySettingsInput struct {
	Server   string   `json:"server"`
	Bypass   []string `json:"bypass"`
	Username string   `json:"username"`
	Password *string  `json:"password,omitempty"`
}
type profileSettingsInput struct {
	Proxy     *proxySettingsInput `json:"proxy"`
	UserAgent string              `json:"user_agent"`
	Lang      string              `json:"lang"`
	Args      []string            `json:"args"`
}

// The private disk index embeds the public profile, but secrets are never part
// of profile's JSON representation or any HTTP response.
type storedProfile struct {
	profile
	ProxyPassword string `json:"proxy_password,omitempty"`
}

var launchValueArgs = map[string]*regexp.Regexp{
	"--window-size":               regexp.MustCompile(`^[0-9]{2,5},[0-9]{2,5}$`),
	"--force-device-scale-factor": regexp.MustCompile(`^(?:[0-3](?:\.[0-9]{1,2})?|4)$`),
	"--disable-features":          regexp.MustCompile(`^[A-Za-z0-9_]+(?:,[A-Za-z0-9_]+)*$`),
	"--enable-features":           regexp.MustCompile(`^[A-Za-z0-9_]+(?:,[A-Za-z0-9_]+)*$`),
	"--blink-settings":            regexp.MustCompile(`^imagesEnabled=(?:true|false)$`),
}

func launchArgOK(arg string) bool {
	switch arg {
	case "--start-maximized", "--disable-gpu", "--disable-background-timer-throttling", "--disable-backgrounding-occluded-windows", "--disable-renderer-backgrounding":
		return true
	}
	name, value, ok := strings.Cut(arg, "=")
	return ok && launchValueArgs[name] != nil && launchValueArgs[name].MatchString(value)
}

var langPattern = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)
var proxyHostPattern = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)

func proxyURL(server string) (*url.URL, error) {
	u, err := url.Parse(server)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(server, "#") || u.Opaque != "" {
		return nil, errors.New("invalid_argument")
	}
	host, port, err := net.SplitHostPort(u.Host)
	n, e := strconv.Atoi(port)
	if err != nil || e != nil || n < 1 || n > 65535 || (net.ParseIP(host) == nil && !proxyHostPattern.MatchString(host)) {
		return nil, errors.New("invalid_argument")
	}
	return u, nil
}
func printable(value string, limit int) bool {
	if len(value) > limit {
		return false
	}
	for _, c := range value {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
func validatedSettings(in profileSettingsInput, previous *proxySettings, password string) (profileSettings, string, error) {
	out := profileSettings{UserAgent: in.UserAgent, Lang: in.Lang, Args: append([]string{}, in.Args...)}
	invalid := func() (profileSettings, string, error) { return profileSettings{}, "", errors.New("invalid_argument") }
	if !printable(in.UserAgent, 512) || len(in.Lang) > 64 || (in.Lang != "" && !langPattern.MatchString(in.Lang)) || len(in.Args) > 32 {
		return invalid()
	}
	for _, a := range in.Args {
		if len(a) > 1024 || !launchArgOK(a) {
			return invalid()
		}
	}
	if in.Proxy == nil {
		return out, "", nil
	}
	p := in.Proxy
	if _, err := proxyURL(p.Server); err != nil {
		return invalid()
	}
	if len(p.Username) > 256 || strings.ContainsAny(p.Username, "\x00\r\n") || len(p.Bypass) > 64 {
		return invalid()
	}
	for _, rule := range p.Bypass {
		if !printable(rule, 256) || rule == "" || strings.ContainsAny(rule, ";\t ") {
			return invalid()
		}
	}
	if p.Password != nil {
		password = *p.Password
	} else if previous == nil || previous.Server != p.Server || previous.Username != p.Username {
		// Never send a saved secret to a different destination or account.
		password = ""
	}
	if len(password) > 4096 || strings.ContainsAny(password, "\x00\r\n") {
		return invalid()
	}
	// SOCKS5 RFC 1929 credentials are length-prefixed single-byte strings.
	if strings.HasPrefix(p.Server, "socks5:") && (len(p.Username) > 255 || len(password) > 255 || (password != "" && p.Username == "")) {
		return invalid()
	}
	out.Proxy = &proxySettings{Server: p.Server, Bypass: append([]string{}, p.Bypass...), Username: p.Username, HasPassword: password != ""}
	return out, password, nil
}
func (e *engine) configureProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Settings *profileSettingsInput `json:"settings"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var trailing any
	if decoder.Decode(&in) != nil || decoder.Decode(&trailing) != io.EOF {
		fail(w, "invalid_argument", 400)
		return
	}
	if in.Settings == nil {
		fail(w, "invalid_argument", 400)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.profileDeleting[r.PathValue("id")] {
		fail(w, "busy", 409)
		return
	}
	p, ok := e.profiles[r.PathValue("id")]
	if !ok {
		fail(w, "not_found", 404)
		return
	}
	settings, password, err := validatedSettings(*in.Settings, p.Settings.Proxy, e.passwords[p.ID])
	if err != nil {
		fail(w, "invalid_argument", 400)
		return
	}
	old, oldPassword := p, e.passwords[p.ID]
	p.Settings = settings
	if e.passwords == nil {
		e.passwords = map[string]string{}
	}
	e.profiles[p.ID], e.passwords[p.ID] = p, password
	if e.saveProfilesLocked() != nil {
		e.profiles[p.ID], e.passwords[p.ID] = old, oldPassword
		fail(w, "storage_failed", 500)
		return
	}
	p.Running = e.sessions[p.ID] != nil
	jsonReply(w, p)
}

// User flags follow the defaults so approved window-size overrides take effect.
func profileLaunchArgs(settings profileSettings, localProxy string) []string {
	args := append([]string{}, settings.Args...)
	if settings.UserAgent != "" {
		args = append(args, "--user-agent="+settings.UserAgent)
	}
	if settings.Lang != "" {
		args = append(args, "--lang="+settings.Lang, "--accept-lang="+settings.Lang)
	}
	if settings.Proxy != nil {
		args = append(args, "--force-webrtc-ip-handling-policy=disable_non_proxied_udp", "--proxy-server="+localProxy, "--proxy-bypass-list="+strings.Join(settings.Proxy.Bypass, ";"))
	}
	return args
}
