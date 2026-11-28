// Package oscap is what an app (a page open in the myco App) can use of the
// computer the App runs on: system information, files, commands, the
// clipboard, notifications, screenshots and the network. The App's backend
// (internal/console) runs these for the page's bridge (myco.os.*).
//
// Each capability has a name ("fs.list"), takes JSON arguments and returns a
// JSON value. Long ones (fs.scan, exec.spawn) run as jobs: they send events
// (progress, output) while they run, can be cancelled, and end with a value
// (jobs.go). What differs between macOS, Windows and Linux is in
// os_darwin.go, os_windows.go and os_linux.go (os_other.go elsewhere); what a
// system cannot do answers "unsupported".
package oscap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/myco-ai/myco/internal/sshcap"
)

// Error is a capability's error: a code the page can act on and a message.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// The codes.
const (
	CodeInvalid     = "invalid_argument"
	CodeNotFound    = "not_found"
	CodeExists      = "exists"
	CodePermission  = "permission_denied"
	CodeUnsupported = "unsupported"
	CodeTimeout     = "timeout"
	CodeCancelled   = "cancelled"
	CodeTooLarge    = "too_large"
	CodeFailed      = "failed"
	// A cloud service has no key here, or the key's account has not turned
	// the service on (speech.transcribe with a cloud engine).
	CodeNoCredentials     = "no_credentials"
	CodeServiceNotEnabled = "service_not_enabled"
)

func errf(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}

func unsupported(what string) *Error {
	return errf(CodeUnsupported, "%s is not available on %s", what, osName())
}

// AsError turns any error into an *Error, with a code from what it is.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	switch {
	case errors.Is(err, context.Canceled):
		return &Error{Code: CodeCancelled, Message: "cancelled"}
	case errors.Is(err, context.DeadlineExceeded):
		return &Error{Code: CodeTimeout, Message: "timed out"}
	case errors.Is(err, fs.ErrNotExist):
		return &Error{Code: CodeNotFound, Message: err.Error()}
	case errors.Is(err, fs.ErrExist):
		return &Error{Code: CodeExists, Message: err.Error()}
	case errors.Is(err, fs.ErrPermission):
		return &Error{Code: CodePermission, Message: err.Error()}
	}
	return &Error{Code: CodeFailed, Message: err.Error()}
}

// Emit sends an event of a job ("progress", "stdout", …). Calls that are not
// jobs get one that drops them.
type Emit func(kind string, data any)

// Func runs a capability.
type Func func(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error)

// Cap is one capability.
type Cap struct {
	Name string
	// Job: it runs as a job (events, cancel); otherwise a single call.
	Job bool
	// What it does, in a few words (for myco.os.list and the docs).
	About string
	// Where it works: nil means everywhere; otherwise why not here ("" when it does).
	Missing func(env *Env) string
	Fn      Func
}

// Secrets keeps local app credentials in the system keychain.
type Secrets interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}

// Env is what capabilities need from the App.
type Env struct {
	SSH     *sshcap.Manager
	Secrets Secrets
	// Scope separates controller profiles by workspace and page.
	Scope string
	// CredentialAudit records no query values, concrete private paths, or secrets.
	CredentialAudit func(origin, method, pathRule string, status int)
	// Home is the person's home folder ("~").
	Home string
	// Path is the PATH commands run with (a GUI app's own is minimal: the
	// login shell's, from the App). "" keeps the process's.
	Path func() string
	// CloudSpeech: speech to text by a cloud service with the App's own key
	// (speech.transcribe with engine "doubao"); nil where the App has none.
	CloudSpeech CloudSpeech
}

// CloudSpeech is speech recognition by a cloud service, set up in the App
// (its key never reaches the page).
type CloudSpeech interface {
	// Status says whether the engine can be used: "" when it can, else an
	// error code (CodeNoCredentials, CodeUnsupported) and why.
	Status(engine string) (code, why string)
	// Transcribe recognizes a 16 kHz mono 16-bit WAV (a few minutes at
	// most) in lang ("" for Chinese with English mixed in); times are
	// seconds from its start.
	Transcribe(ctx context.Context, engine string, wav []byte, lang string) ([]Segment, error)
}

// NewEnv is an Env for this person.
func NewEnv(path func() string) *Env {
	home, _ := os.UserHomeDir()
	return &Env{Home: home, Path: path, SSH: &sshcap.Manager{Home: home}}
}

// PATH is the PATH for commands.
func (e *Env) PATH() string {
	if e != nil && e.Path != nil {
		if p := e.Path(); p != "" {
			return p
		}
	}
	return os.Getenv("PATH")
}

// LookPath finds a program on PATH().
func (e *Env) LookPath(name string) (string, error) {
	if strings.ContainsAny(name, `/\`) {
		return exec.LookPath(name)
	}
	for _, dir := range filepath.SplitList(e.PATH()) {
		if dir == "" {
			continue
		}
		for _, cand := range exeNames(filepath.Join(dir, name)) {
			if st, err := os.Stat(cand); err == nil && !st.IsDir() && (runtime.GOOS == "windows" || st.Mode()&0o111 != 0) {
				return cand, nil
			}
		}
	}
	return exec.LookPath(name)
}

// Environ is the environment for commands: the App's, with PATH() and
// overrides.
func (e *Env) Environ(over map[string]string) []string {
	out := make([]string, 0, len(os.Environ())+len(over)+1)
	path := e.PATH()
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "PATH") {
			continue
		}
		if _, ok := over[k]; ok {
			continue
		}
		// (the App's own secrets stay with the App)
		if k == "MYCO_TOKEN" || strings.HasPrefix(k, "MYCO_HUB_TOKEN") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "PATH="+path)
	keys := make([]string, 0, len(over))
	for k := range over {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.EqualFold(k, "PATH") {
			out[len(out)-1] = "PATH=" + over[k]
			continue
		}
		out = append(out, k+"="+over[k])
	}
	return out
}

var byName = map[string]*Cap{}

func register(cs ...*Cap) {
	for _, c := range cs {
		if _, dup := byName[c.Name]; dup {
			panic("oscap: " + c.Name + " twice")
		}
		byName[c.Name] = c
	}
}

// Info is a capability as myco.os.list shows it.
type Info struct {
	Name  string `json:"name"`
	Job   bool   `json:"job,omitempty"`
	About string `json:"about"`
	// Available here; when not, Missing says why.
	Available bool   `json:"available"`
	Missing   string `json:"missing,omitempty"`
}

// List is every capability, and whether it works on this computer.
func List(env *Env) []Info {
	out := make([]Info, 0, len(byName))
	for _, c := range byName {
		i := Info{Name: c.Name, Job: c.Job, About: c.About, Available: true}
		if c.Missing != nil {
			if why := c.Missing(env); why != "" {
				i.Available, i.Missing = false, why
			}
		}
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// Lookup is the capability with this name.
func Lookup(name string) (*Cap, bool) {
	c, ok := byName[name]
	return c, ok
}

// Call runs a capability that is not a job.
func Call(ctx context.Context, env *Env, name string, args json.RawMessage) (any, *Error) {
	c, ok := byName[name]
	if !ok {
		return nil, errf(CodeInvalid, "no such capability: %q", name)
	}
	if c.Job {
		return nil, errf(CodeInvalid, "%s runs as a job: start it (myco.os.start)", name)
	}
	v, err := run(ctx, env, c, args, func(string, any) {})
	return v, AsError(err)
}

func run(ctx context.Context, env *Env, c *Cap, args json.RawMessage, emit Emit) (v any, err error) {
	if c.Missing != nil {
		if why := c.Missing(env); why != "" {
			return nil, errf(CodeUnsupported, "%s", why)
		}
	}
	defer func() {
		if p := recover(); p != nil {
			err = errf(CodeFailed, "%s failed: %v", c.Name, p)
		}
	}()
	return c.Fn(ctx, env, args, emit)
}

// decode reads arguments into v; empty arguments leave v as it is.
func decode(args json.RawMessage, v any) error {
	if len(args) == 0 || string(args) == "null" {
		return nil
	}
	if err := json.Unmarshal(args, v); err != nil {
		return errf(CodeInvalid, "arguments: %v", err)
	}
	return nil
}

// osName names this system in messages.
func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return runtime.GOOS
}
