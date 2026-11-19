//go:build !darwin && !windows

package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// guiShell is what /app/info reports for the window.
const guiShell = "browser"

//go:embed build/linux/icon-256.png
var appIcon []byte

// On Linux the App is this program serving the console on a loopback port,
// opened in the default browser: no window toolkit, so it runs the same
// under X11 and Wayland. One copy runs: starting it again (from the
// launcher) opens the running one. The port and the key are kept, so a
// tab left open keeps working after a restart or an update. myco:// links
// are not registered (as on Windows).
func runGUI(h http.Handler, _ func(string), setRelaunch func(func())) error {
	path, err := windowStatePath()
	if err != nil {
		return err
	}
	st := loadWindowState(path)
	relaunched := os.Getenv("MYCO_APP_RELAUNCHED") == "1"
	os.Unsetenv("MYCO_APP_RELAUNCHED")
	if !relaunched && st.Key != "" && st.alive() {
		fmt.Println("myco is already running:", st.url())
		if err := openBrowser(st.url()); err != nil {
			fmt.Println("open the address above in your browser")
		}
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", st.Port))
	if err != nil && st.Port != 0 {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return err
	}
	st.Port = ln.Addr().(*net.TCPAddr).Port
	if st.Key == "" {
		k := make([]byte, 16)
		_, _ = rand.Read(k)
		st.Key = hex.EncodeToString(k)
	}
	if err := st.save(path); err != nil {
		ln.Close()
		return err
	}
	openURL = openBrowser
	// An update put a new program in place: run it in this process, which
	// gives the port back to it; the tab stays as it is. The path is read
	// now: once the update renamed this program, os.Executable names that.
	exe, exeErr := os.Executable()
	setRelaunch(func() {
		err := exeErr
		if err == nil {
			err = syscall.Exec(exe, os.Args, append(os.Environ(), "MYCO_APP_RELAUNCHED=1"))
		}
		fmt.Fprintln(os.Stderr, "myco-app: could not restart, start it again:", err)
		os.Exit(1)
	})
	if err := installLauncher(); err != nil {
		fmt.Fprintln(os.Stderr, "myco-app: could not add myco to the applications menu:", err)
	}
	fmt.Println("myco:", st.url())
	if !relaunched {
		go func() {
			if err := openBrowser(st.url()); err != nil {
				fmt.Println("open the address above in your browser")
			}
		}()
	}
	return http.Serve(ln, guarded(quitHandler(h), st.Port, st.Key))
}

// quitHandler adds POST /app/quit (behind the key, like every page): the
// app stops, as quitting from the tray does elsewhere.
func quitHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/app/quit" {
			w.WriteHeader(http.StatusNoContent)
			go func() { time.Sleep(200 * time.Millisecond); os.Exit(0) }()
			return
		}
		h.ServeHTTP(w, r)
	})
}

// quitRunning stops the copy of the app that is running (myco-app --quit).
func quitRunning() error {
	path, err := windowStatePath()
	if err != nil {
		return err
	}
	st := loadWindowState(path)
	if st.Key == "" || !st.alive() {
		fmt.Println("myco is not running")
		return nil
	}
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/app/quit", st.Port), nil)
	req.AddCookie(&http.Cookie{Name: serveCookie, Value: st.Key})
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the running app answered %s", resp.Status)
	}
	fmt.Println("myco stopped")
	return nil
}

// windowState is where the running app listens and its key (0600: the key
// opens the console).
type windowState struct {
	Port int    `json:"port"`
	Key  string `json:"key"`
}

func windowStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "myco", "app-window.json"), nil
}

func loadWindowState(path string) windowState {
	var st windowState
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Port < 0 || st.Port > 65535 {
		st.Port = 0
	}
	return st
}

func (st windowState) save(path string) error {
	b, _ := json.Marshal(st)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (st windowState) url() string {
	return fmt.Sprintf("http://127.0.0.1:%d/?key=%s", st.Port, st.Key)
}

// alive says whether a copy of the app answers on the saved port with the
// saved key (it trades the key for its cookie with a redirect).
func (st windowState) alive() bool {
	c := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(st.url())
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusFound && resp.Header.Get("Location") == "/"
}

// openBrowser opens u in the default browser, without waiting for it.
func openBrowser(u string) error {
	var cmds [][]string
	if b := os.Getenv("BROWSER"); b != "" {
		cmds = append(cmds, []string{b})
	}
	cmds = append(cmds, []string{"xdg-open"}, []string{"gio", "open"}, []string{"wslview"})
	for _, c := range cmds {
		p, err := exec.LookPath(c[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(p, append(c[1:], u)...)
		if err := cmd.Start(); err != nil {
			continue
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}
	return errors.New("no browser found (xdg-open)")
}

// installLauncher puts myco in the applications menu: a desktop entry that
// starts this program, and its icon (the XDG data directory, usually
// ~/.local/share). It is written again when the program moved.
func installLauncher() error {
	if version == "dev" {
		return nil // a development build: not in the menu
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		data = filepath.Join(home, ".local", "share")
	}
	icon := filepath.Join(data, "icons", "hicolor", "256x256", "apps", "myco.png")
	if err := writeIfChanged(icon, appIcon, 0o644); err != nil {
		return err
	}
	return writeIfChanged(filepath.Join(data, "applications", "myco.desktop"), []byte(desktopEntry(exe)), 0o644)
}

// desktopEntry is the launcher for exe (quoted as the spec asks).
func desktopEntry(exe string) string {
	q := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", "$", `\\$`, "%", "%%").Replace(exe)
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=myco\n" +
		"Comment=All your AI agents, one app\n" +
		"Comment[zh_CN]=你的 AI agent，一个 App 就够\n" +
		"Exec=\"" + q + "\"\n" +
		"Icon=myco\n" +
		"Terminal=false\n" +
		"Categories=Development;Utility;\n" +
		"StartupNotify=false\n"
}

func writeIfChanged(path string, b []byte, mode os.FileMode) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(b) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, mode)
}
