package console

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/oscap"
)

// What a page (an app) can use of this computer: myco.os.* in its bridge,
// answered here by internal/oscap. Each call is written to
// ~/.myco/logs/app-os.jsonl.
//
//	GET    /app/os                    what there is, and what works here
//	POST   /app/os/call               {name, args, page} → the value
//	POST   /app/os/jobs               {name, args, page} → {id}: a job (fs.scan, exec.spawn)
//	GET    /app/os/jobs/{id}?after=n  its events after the n-th (waits up to 20 s for one), and its end
//	POST   /app/os/jobs/{id}/input    {data, encoding, close}: what it reads (exec.spawn)
//	DELETE /app/os/jobs/{id}          stop it

// The most a call's body can be (fs.write of a large file, as base64).
const maxOSBody = 96 << 20

type osState struct {
	once sync.Once
	env  *oscap.Env
	jobs *oscap.Jobs
	logM sync.Mutex
}

func (b *Backend) osInit() (*oscap.Env, *oscap.Jobs) {
	s := &b.os
	s.once.Do(func() {
		var path func() string
		if b.Local != nil {
			path = b.Local.loginShellPath
		}
		s.env = oscap.NewEnv(path)
		s.env.Secrets = b.Secrets
		if b.Local != nil && b.Local.Home != "" {
			s.env.Home = b.Local.Home
			s.env.SSH.Home = b.Local.Home
		}
		s.jobs = &oscap.Jobs{Env: s.env}
	})
	return s.env, s.jobs
}

// osPage is the page that called (for the log).
type osPage struct {
	Hub  string `json:"hub,omitempty"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type osCallIn struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
	Page osPage          `json:"page"`
}

func osStatus(code string) int {
	switch code {
	case oscap.CodeInvalid:
		return http.StatusBadRequest
	case oscap.CodeNotFound:
		return http.StatusNotFound
	case oscap.CodeExists, oscap.CodeCancelled, "conflict":
		return http.StatusConflict
	case oscap.CodePermission:
		return http.StatusForbidden
	case oscap.CodeUnsupported:
		return http.StatusNotImplemented
	case oscap.CodeTimeout:
		return http.StatusGatewayTimeout
	case oscap.CodeTooLarge:
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusInternalServerError
}

func writeOSErr(w http.ResponseWriter, e *oscap.Error) {
	writeErr(w, &client.Error{Status: osStatus(e.Code), Code: e.Code, Message: e.Message})
}

func (b *Backend) osRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/os", func(w http.ResponseWriter, r *http.Request) {
		env, _ := b.osInit()
		writeJSON(w, http.StatusOK, map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "caps": oscap.List(env)})
	})
	m.HandleFunc("POST /app/os/call", func(w http.ResponseWriter, r *http.Request) {
		var in osCallIn
		r.Body = http.MaxBytesReader(w, r.Body, maxOSBody)
		if !decode(w, r, &in) {
			return
		}
		if (in.Name == "proxy.profile" || in.Name == "proxy.control" || strings.HasPrefix(in.Name, "db.")) && (in.Page.ID == "" || in.Page.Hub == "") {
			writeOSErr(w, &oscap.Error{Code: oscap.CodeInvalid, Message: "controller calls require a workspace and page"})
			return
		}
		env, _ := b.osInit()
		start := time.Now()
		scoped := *env
		scoped.Scope = in.Page.Hub + "\x00" + in.Page.ID
		v, err := oscap.Call(r.Context(), &scoped, in.Name, in.Args)
		b.osLog(in, "", start, err)
		if err == nil && in.Name == "ssh.profiles" && b.Local != nil && b.Store != nil {
			go b.reconcileActionHosts()
		}
		if err != nil {
			writeOSErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"value": v})
	})
	m.HandleFunc("POST /app/os/jobs", func(w http.ResponseWriter, r *http.Request) {
		var in osCallIn
		r.Body = http.MaxBytesReader(w, r.Body, maxOSBody)
		if !decode(w, r, &in) {
			return
		}
		_, jobs := b.osInit()
		j, err := jobs.Start(in.Name, in.Args)
		id := ""
		if j != nil {
			id = j.ID
		}
		b.osLog(in, id, time.Now(), err)
		if err != nil {
			writeOSErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": j.ID})
	})
	m.HandleFunc("GET /app/os/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, jobs := b.osInit()
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		wait := 20 * time.Second
		if s := r.URL.Query().Get("wait"); s != "" {
			if ms, err := strconv.Atoi(s); err == nil && ms >= 0 && ms < 60000 {
				wait = time.Duration(ms) * time.Millisecond
			}
		}
		batch, err := jobs.Read(r.Context(), r.PathValue("id"), after, wait)
		if err != nil {
			writeOSErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, batch)
	})
	m.HandleFunc("POST /app/os/jobs/{id}/input", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Data     string `json:"data"`
			Encoding string `json:"encoding"`
			Close    bool   `json:"close"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxOSBody)
		if !decode(w, r, &in) {
			return
		}
		_, jobs := b.osInit()
		data := []byte(in.Data)
		if in.Encoding == "base64" {
			var err error
			if data, err = base64.StdEncoding.DecodeString(in.Data); err != nil {
				writeOSErr(w, &oscap.Error{Code: oscap.CodeInvalid, Message: "data is not base64"})
				return
			}
		}
		if len(data) > 0 {
			if err := jobs.Input(r.PathValue("id"), data); err != nil {
				writeOSErr(w, err)
				return
			}
		}
		if in.Close {
			if err := jobs.Input(r.PathValue("id"), nil); err != nil {
				writeOSErr(w, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	m.HandleFunc("DELETE /app/os/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, jobs := b.osInit()
		if err := jobs.Cancel(r.PathValue("id")); err != nil {
			writeOSErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

// osLog writes a line for each call: which page, what, how it went. The
// arguments are never recorded: even a short prefix can expose a credential.
func (b *Backend) osLog(in osCallIn, job string, start time.Time, err *oscap.Error) {
	if b.Local == nil || b.Local.Home == "" {
		return
	}
	// Capability arguments can contain credentials, file contents and shell
	// commands. Keep their size, never their contents, in the audit log.
	line := map[string]any{"at": start.UTC().Format(time.RFC3339Nano), "page": in.Page, "name": in.Name, "args_bytes": len(in.Args), "ms": time.Since(start).Milliseconds()}
	if job != "" {
		line["job"] = job
	}
	if err != nil {
		line["error"] = err.Code
	}
	b.os.logM.Lock()
	defer b.os.logM.Unlock()
	dir := filepath.Join(b.Local.Home, ".myco", "logs")
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	file := filepath.Join(dir, "app-os.jsonl")
	if st, err := os.Stat(file); err == nil && st.Size() > 5<<20 {
		_ = os.Rename(file, file+".1")
	}
	f, err2 := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err2 != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(line)
}
