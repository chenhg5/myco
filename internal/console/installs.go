package console

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/client"
)

// Pages installed as apps (App v0.22.11): a launcher on this computer opens
// the page in a window of its own, from where the system starts apps. On
// macOS it is a small bundle in ~/Applications/myco 应用/ (Launchpad,
// Spotlight, the Dock) that opens myco://app?hub=&page=&name=; the App
// opens the page's window (pagewin_gui.go). The list is kept next to the
// App's hubs.json.
//
//	GET    /app/installs                          {supported, installs}
//	POST   /app/hubs/{id}/pages/{pid}/install     {name, icon (a PNG data URL)} → the install
//	DELETE /app/hubs/{id}/pages/{pid}/install

type installedApp struct {
	Hub  string `json:"hub"`
	Page string `json:"page"`
	Name string `json:"name"`
	// Path: the launcher on this computer
	Path string `json:"path"`
	At   int64  `json:"at"`
}

type installs struct {
	mu   sync.Mutex
	file string
	list []installedApp
}

func (b *Backend) installs() *installs {
	b.installsOnce.Do(func() {
		in := &installs{}
		if b.Store != nil && b.Store.path != "" {
			in.file = filepath.Join(filepath.Dir(b.Store.path), "installs.json")
			if raw, err := os.ReadFile(in.file); err == nil {
				_ = json.Unmarshal(raw, &in.list)
			}
		}
		b.installed = in
	})
	return b.installed
}

func (in *installs) saveLocked() {
	if in.file == "" {
		return
	}
	raw, _ := json.MarshalIndent(in.list, "", "  ")
	if os.MkdirAll(filepath.Dir(in.file), 0o700) == nil {
		_ = os.WriteFile(in.file, append(raw, '\n'), 0o600)
	}
}

// launcherDir is where launchers go ("" where the App cannot make them).
func (b *Backend) launcherDir() string {
	if launcherKind == "" || b.OpenPageWindow == nil {
		return ""
	}
	home := ""
	if b.Local != nil && b.Local.Home != "" {
		home = b.Local.Home
	} else if h, err := os.UserHomeDir(); err == nil {
		home = h
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Applications", "myco 应用")
}

// appLink is what a launcher opens.
func appLink(hub, page, name string) string {
	return "myco://app?" + url.Values{"hub": {hub}, "page": {page}, "name": {name}}.Encode()
}

// fileName makes an app's name safe as a file name.
func fileName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	name = strings.Trim(name, ". ")
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	if name == "" {
		name = "myco app"
	}
	return name
}

// icns wraps a PNG (512 px) as a macOS icon file.
func icns(png []byte) []byte {
	entry := make([]byte, 8, 8+len(png))
	copy(entry, "ic09")
	binary.BigEndian.PutUint32(entry[4:], uint32(8+len(png)))
	entry = append(entry, png...)
	out := make([]byte, 8, 8+len(entry))
	copy(out, "icns")
	binary.BigEndian.PutUint32(out[4:], uint32(8+len(entry)))
	return append(out, entry...)
}

// writeMacBundle makes the launcher bundle <dir>/<name>.app: a script that
// opens the app's link (the App opens its window), with the app's icon.
// It has no Dock tile of its own while it runs (LSUIElement): the window
// it asks for is the App's.
func writeMacBundle(dir, name, hub, page string, png []byte) (string, error) {
	sum := sha256.Sum256([]byte(hub + "\x00" + page))
	path := filepath.Join(dir, fileName(name)+".app")
	contents := filepath.Join(path, "Contents")
	if err := os.MkdirAll(filepath.Join(contents, "MacOS"), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(contents, "Resources"), 0o755); err != nil {
		return "", err
	}
	esc := func(s string) string { return html.EscapeString(s) }
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>` + esc(name) + `</string>
	<key>CFBundleDisplayName</key><string>` + esc(name) + `</string>
	<key>CFBundleIdentifier</key><string>dev.myco.app.` + hex.EncodeToString(sum[:8]) + `</string>
	<key>CFBundleExecutable</key><string>launch</string>
	<key>CFBundleIconFile</key><string>icon</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleVersion</key><string>1</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>LSUIElement</key><true/>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0o644); err != nil {
		return "", err
	}
	// (the link in single quotes: it has none of its own, being URL-encoded)
	script := "#!/bin/sh\n# Opens this app in myco (installed from the myco App).\nexec /usr/bin/open '" + appLink(hub, page, name) + "'\n"
	if err := os.WriteFile(filepath.Join(contents, "MacOS", "launch"), []byte(script), 0o755); err != nil {
		return "", err
	}
	if len(png) > 0 {
		if err := os.WriteFile(filepath.Join(contents, "Resources", "icon.icns"), icns(png), 0o644); err != nil {
			return "", err
		}
	}
	return path, nil
}

// iconPNG reads a PNG data URL (at most 1 MB).
func iconPNG(dataURL string) ([]byte, error) {
	if dataURL == "" {
		return nil, nil
	}
	raw, ok := strings.CutPrefix(dataURL, "data:image/png;base64,")
	if !ok || len(raw) > 1400000 {
		return nil, fmt.Errorf("icon: a PNG data URL of at most 1 MB")
	}
	png, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(png) < 8 || string(png[1:4]) != "PNG" {
		return nil, fmt.Errorf("icon: not a PNG")
	}
	return png, nil
}

func (b *Backend) installRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/installs", func(w http.ResponseWriter, r *http.Request) {
		in := b.installs()
		in.mu.Lock()
		list := append([]installedApp{}, in.list...)
		in.mu.Unlock()
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		writeJSON(w, http.StatusOK, map[string]any{"supported": b.launcherDir() != "", "installs": list})
	})
	m.HandleFunc("POST /app/hubs/{id}/pages/{pid}/install", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
			Icon string `json:"icon"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		if !decode(w, r, &body) {
			return
		}
		dir := b.launcherDir()
		if dir == "" {
			writeErr(w, &client.Error{Status: http.StatusNotImplemented, Code: "unsupported", Message: "pages are installed as apps only in the desktop App on macOS"})
			return
		}
		hub, page := r.PathValue("id"), r.PathValue("pid")
		if _, err := b.find(hub); err != nil {
			writeErr(w, err)
			return
		}
		png, err := iconPNG(body.Icon)
		if err != nil {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error()})
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = "myco app"
		}
		in := b.installs()
		in.mu.Lock()
		defer in.mu.Unlock()
		// again: the old launcher goes (its name may have changed)
		rest := in.list[:0]
		for _, e := range in.list {
			if e.Hub == hub && e.Page == page {
				removeLauncher(dir, e.Path)
				continue
			}
			rest = append(rest, e)
		}
		in.list = rest
		path, err := writeMacBundle(dir, name, hub, page, png)
		if err != nil {
			writeErr(w, err)
			return
		}
		e := installedApp{Hub: hub, Page: page, Name: name, Path: path, At: time.Now().Unix()}
		in.list = append(in.list, e)
		in.saveLocked()
		writeJSON(w, http.StatusOK, e)
	})
	m.HandleFunc("DELETE /app/hubs/{id}/pages/{pid}/install", func(w http.ResponseWriter, r *http.Request) {
		hub, page := r.PathValue("id"), r.PathValue("pid")
		dir := b.launcherDir()
		in := b.installs()
		in.mu.Lock()
		defer in.mu.Unlock()
		rest := in.list[:0]
		for _, e := range in.list {
			if e.Hub == hub && e.Page == page {
				removeLauncher(dir, e.Path)
				continue
			}
			rest = append(rest, e)
		}
		in.list = rest
		in.saveLocked()
		w.WriteHeader(http.StatusNoContent)
	})
}

// removeLauncher deletes a launcher this App made: a bundle in its own
// folder, nothing else.
func removeLauncher(dir, path string) {
	if dir == "" || path == "" || !strings.HasSuffix(path, ".app") || filepath.Dir(path) != filepath.Clean(dir) {
		return
	}
	_ = os.RemoveAll(path)
}
