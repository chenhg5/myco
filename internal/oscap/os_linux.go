package oscap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Linux: /proc and /etc/os-release for the system, the freedesktop trash,
// and whichever of the usual programs is installed (xdg-open, wl-copy or
// xclip, notify-send, grim or gnome-screenshot) for the rest.

func isHidden(_ string, fi fs.FileInfo) bool { return strings.HasPrefix(fi.Name(), ".") }

// userDirs reads ~/.config/user-dirs.dirs (XDG_DOWNLOAD_DIR="$HOME/下载").
func userDirs(home string) map[string]string {
	out := map[string]string{
		"desktop": filepath.Join(home, "Desktop"), "documents": filepath.Join(home, "Documents"), "downloads": filepath.Join(home, "Downloads"),
		"pictures": filepath.Join(home, "Pictures"), "music": filepath.Join(home, "Music"), "videos": filepath.Join(home, "Videos"),
		"trash": trashHome(home),
	}
	keys := map[string]string{"XDG_DESKTOP_DIR": "desktop", "XDG_DOCUMENTS_DIR": "documents", "XDG_DOWNLOAD_DIR": "downloads",
		"XDG_PICTURES_DIR": "pictures", "XDG_MUSIC_DIR": "music", "XDG_VIDEOS_DIR": "videos"}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	f, err := os.Open(filepath.Join(cfg, "user-dirs.dirs"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || keys[k] == "" {
			continue
		}
		v = strings.Trim(v, `"`)
		v = strings.Replace(v, "$HOME", home, 1)
		if filepath.IsAbs(v) {
			out[keys[k]] = v
		}
	}
	return out
}

func trashHome(home string) string {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "Trash")
}

func fillInfo(i *SystemInfo) {
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			k, v, _ := strings.Cut(l, "=")
			v = strings.Trim(v, `"`)
			switch k {
			case "PRETTY_NAME":
				i.Name = v
			case "VERSION_ID":
				i.Version = v
			}
		}
	}
	if i.Version == "" {
		var u unix.Utsname
		if unix.Uname(&u) == nil {
			i.Version = cstrLinux(u.Release[:])
		}
	}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			k, v, _ := strings.Cut(l, ":")
			if k = strings.TrimSpace(k); k == "model name" || k == "Model" {
				i.CPUModel = strings.TrimSpace(v)
				break
			}
		}
	}
	if b, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_name"); err == nil {
		i.Model = strings.TrimSpace(string(b))
	}
	var si unix.Sysinfo_t
	if unix.Sysinfo(&si) == nil {
		i.Memory = uint64(si.Totalram) * uint64(si.Unit)
		i.Uptime = int64(si.Uptime)
	}
}

func cstrLinux(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// Not disks: the kernel's own file systems.
var pseudoFS = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true, "tmpfs": true, "cgroup": true, "cgroup2": true, "securityfs": true,
	"pstore": true, "debugfs": true, "tracefs": true, "mqueue": true, "hugetlbfs": true, "fusectl": true, "configfs": true, "bpf": true,
	"autofs": true, "binfmt_misc": true, "nsfs": true, "ramfs": true, "rpc_pipefs": true, "squashfs": true, "efivarfs": true,
	"fuse.gvfsd-fuse": true, "fuse.portal": true, "selinuxfs": true, "overlay": true,
}

type mountLine struct{ dev, mount, fs, opts string }

func mounts() []mountLine {
	b, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return nil
	}
	var out []mountLine
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 4 {
			continue
		}
		// (spaces in a mount point are written \040)
		out = append(out, mountLine{dev: f[0], mount: strings.ReplaceAll(f[1], `\040`, " "), fs: f[2], opts: f[3]})
	}
	return out
}

func disks() ([]Disk, error) {
	var out []Disk
	seen := map[string]bool{}
	for _, m := range mounts() {
		if pseudoFS[m.fs] && !(m.fs == "overlay" && m.mount == "/") || strings.HasPrefix(m.mount, "/snap/") || strings.HasPrefix(m.mount, "/proc") || strings.HasPrefix(m.mount, "/sys") {
			continue
		}
		if seen[m.dev] && m.dev != "overlay" {
			continue
		}
		var s unix.Statfs_t
		if unix.Statfs(m.mount, &s) != nil || s.Blocks == 0 {
			continue
		}
		seen[m.dev] = true
		bs := uint64(s.Bsize)
		d := Disk{Mount: m.mount, Device: m.dev, FS: m.fs, Label: filepath.Base(m.mount), Total: s.Blocks * bs, Free: s.Bfree * bs, Available: s.Bavail * bs}
		d.ReadOnly = strings.HasPrefix(m.opts, "ro,") || m.opts == "ro"
		d.Network = m.fs == "nfs" || m.fs == "nfs4" || m.fs == "cifs" || m.fs == "smbfs" || strings.HasPrefix(m.fs, "fuse.sshfs")
		d.Removable = strings.HasPrefix(m.mount, "/media/") || strings.HasPrefix(m.mount, "/run/media/")
		if m.mount == "/" {
			d.Label = "/"
		}
		out = append(out, d)
	}
	return out, nil
}

func isVolumeRoot(p string) bool {
	for _, m := range mounts() {
		if m.mount == p && !pseudoFS[m.fs] {
			return true
		}
	}
	return false
}

// trash: the freedesktop.org trash in the home folder (what Files,
// Dolphin and the others show), with the .trashinfo they read to put
// things back.
func trash(_ context.Context, env *Env, paths []string) []error {
	errs := make([]error, len(paths))
	dir := trashHome(env.Home)
	for i, p := range paths {
		errs[i] = trashOne(dir, p)
	}
	return errs
}

func trashOne(dir, p string) error {
	files, info := filepath.Join(dir, "files"), filepath.Join(dir, "info")
	if err := os.MkdirAll(files, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(info, 0o700); err != nil {
		return err
	}
	name := filepath.Base(p)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n < 10000; n++ {
		cand := name
		if n > 1 {
			cand = fmt.Sprintf("%s.%d%s", stem, n, ext)
		}
		// (the .trashinfo claims the name)
		f, err := os.OpenFile(filepath.Join(info, cand+".trashinfo"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		u := (&url.URL{Path: p}).EscapedPath()
		_, err = fmt.Fprintf(f, "[Trash Info]\nPath=%s\nDeletionDate=%s\n", u, time.Now().Format("2006-01-02T15:04:05"))
		f.Close()
		if err != nil {
			os.Remove(f.Name())
			return err
		}
		to := filepath.Join(files, cand)
		if err := os.Rename(p, to); err != nil {
			// another disk: copy into the home trash, then remove
			if err2 := copyAll(p, to); err2 != nil {
				os.RemoveAll(to)
				os.Remove(filepath.Join(info, cand+".trashinfo"))
				return err
			}
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
		return nil
	}
	return errf(CodeFailed, "no free name in %s", files)
}

func reveal(env *Env, p string) error {
	// the file manager's own "show this item" (Files, Dolphin, Nemo…), else the folder
	if _, err := runOut(context.Background(), env, nil, nil, "dbus-send", "--session", "--print-reply", "--dest=org.freedesktop.FileManager1",
		"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems", "array:string:"+(&url.URL{Scheme: "file", Path: p}).String(), "string:"); err == nil {
		return nil
	}
	_, err := startDetached(env, "xdg-open", filepath.Dir(p))
	return err
}

func open(env *Env, p, app string) error {
	if app != "" {
		_, err := startDetached(env, app, p)
		return err
	}
	_, err := startDetached(env, "xdg-open", p)
	return err
}

// startDetached starts a program and does not wait for it (xdg-open may
// stay until the program it opened quits).
func startDetached(env *Env, name string, args ...string) (int, error) {
	prog, err := env.LookPath(name)
	if err != nil {
		return 0, errf(CodeUnsupported, "%s is not installed", name)
	}
	cmd := exec.Command(prog, args...)
	cmd.Env = env.Environ(nil)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}

func wayland() bool { return os.Getenv("WAYLAND_DISPLAY") != "" }

// The first of these that is installed.
func firstOf(env *Env, names ...string) string {
	for _, n := range names {
		if _, err := env.LookPath(n); err == nil {
			return n
		}
	}
	return ""
}

func clipboardTool(env *Env) string {
	if wayland() {
		if firstOf(env, "wl-copy") != "" && firstOf(env, "wl-paste") != "" {
			return "wl"
		}
	}
	return firstOf(env, "xclip", "xsel")
}

func clipboardMissing(env *Env) string {
	if clipboardTool(env) == "" {
		return "the clipboard needs wl-clipboard (Wayland), xclip or xsel"
	}
	return ""
}

func clipboardGet(ctx context.Context, env *Env) (string, error) {
	var out []byte
	var err error
	switch clipboardTool(env) {
	case "wl":
		out, err = runOut(ctx, env, nil, nil, "wl-paste", "--no-newline")
	case "xclip":
		out, err = runOut(ctx, env, nil, nil, "xclip", "-selection", "clipboard", "-o")
	case "xsel":
		out, err = runOut(ctx, env, nil, nil, "xsel", "--clipboard", "--output")
	default:
		return "", errf(CodeUnsupported, "%s", clipboardMissing(env))
	}
	return string(out), err
}

func clipboardSet(ctx context.Context, env *Env, s string) error {
	var err error
	switch clipboardTool(env) {
	case "wl":
		_, err = runOut(ctx, env, []byte(s), nil, "wl-copy")
	case "xclip":
		_, err = runOut(ctx, env, []byte(s), nil, "xclip", "-selection", "clipboard", "-i")
	case "xsel":
		_, err = runOut(ctx, env, []byte(s), nil, "xsel", "--clipboard", "--input")
	default:
		return errf(CodeUnsupported, "%s", clipboardMissing(env))
	}
	return err
}

func notifyMissing(env *Env) string {
	if firstOf(env, "notify-send") == "" {
		return "notifications need notify-send (libnotify)"
	}
	return ""
}

func notify(ctx context.Context, env *Env, title, subtitle, body string, _ bool) error {
	if title == "" {
		title = "myco"
	}
	if subtitle != "" {
		body = subtitle + "\n" + body
	}
	_, err := runOut(ctx, env, nil, nil, "notify-send", "--app-name=myco", title, body)
	return err
}

func screenTool(env *Env) string {
	if wayland() {
		if t := firstOf(env, "grim", "gnome-screenshot", "spectacle"); t != "" {
			return t
		}
	}
	return firstOf(env, "gnome-screenshot", "spectacle", "import", "scrot", "grim")
}

func screenMissing(env *Env) string {
	if screenTool(env) == "" {
		return "screenshots need grim (Wayland), gnome-screenshot, spectacle, ImageMagick (import) or scrot"
	}
	return ""
}

// capture: the whole screen with whichever tool there is (a region is
// cut out afterwards).
func capture(ctx context.Context, env *Env, file string, _ int, _ *Region) (bool, error) {
	var argv []string
	switch screenTool(env) {
	case "grim":
		argv = []string{"grim", file}
	case "gnome-screenshot":
		argv = []string{"gnome-screenshot", "-f", file}
	case "spectacle":
		argv = []string{"spectacle", "-b", "-n", "-f", "-o", file}
	case "import":
		argv = []string{"import", "-window", "root", file}
	case "scrot":
		argv = []string{"scrot", "-o", file}
	default:
		return false, errf(CodeUnsupported, "%s", screenMissing(env))
	}
	_, err := runOut(ctx, env, nil, nil, argv...)
	return false, err
}

func cpuTimes() (idle, total uint64, ok bool) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	line, _, _ := strings.Cut(string(b), "\n")
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, false
	}
	for i, v := range f[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		total += n
		if i == 3 || i == 4 { // idle, iowait
			idle += n
		}
	}
	return idle, total, true
}

// usage: /proc/stat sampled twice, /proc/meminfo, /proc/loadavg.
func usage(ctx context.Context, _ *Env) (Usage, error) {
	u := Usage{}
	i1, t1, ok := cpuTimes()
	if ok {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return u, ctx.Err()
		}
		if i2, t2, ok := cpuTimes(); ok && t2 > t1 {
			u.CPU = 100 * (1 - float64(i2-i1)/float64(t2-t1))
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		kv := map[string]uint64{}
		for _, l := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			f := strings.Fields(v)
			if len(f) > 0 {
				n, _ := strconv.ParseUint(f[0], 10, 64)
				kv[k] = n * 1024
			}
		}
		u.MemoryTotal, u.MemoryAvailable = kv["MemTotal"], kv["MemAvailable"]
		if u.MemoryTotal > u.MemoryAvailable {
			u.MemoryUsed = u.MemoryTotal - u.MemoryAvailable
		}
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		for _, v := range strings.Fields(string(b))[:3] {
			n, _ := strconv.ParseFloat(v, 64)
			u.Load = append(u.Load, n)
		}
	}
	return u, nil
}

// apps: the .desktop entries menus show (system, Flatpak, Snap, the person's own).
func apps(ctx context.Context, env *Env) ([]AppInfo, error) {
	var out []AppInfo
	seen := map[string]bool{}
	dirs := []string{filepath.Join(env.Home, ".local", "share", "applications"), "/usr/local/share/applications", "/usr/share/applications",
		"/var/lib/flatpak/exports/share/applications", filepath.Join(env.Home, ".local", "share", "flatpak", "exports", "share", "applications"), "/var/lib/snapd/desktop/applications"}
	for _, dir := range dirs {
		des, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range des {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !strings.HasSuffix(de.Name(), ".desktop") || seen[de.Name()] {
				continue
			}
			seen[de.Name()] = true
			p := filepath.Join(dir, de.Name())
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			a := AppInfo{ID: strings.TrimSuffix(de.Name(), ".desktop"), Path: p, System: strings.HasPrefix(p, "/usr/")}
			hidden, entry := false, false
			for _, l := range strings.Split(string(b), "\n") {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "[") {
					entry = l == "[Desktop Entry]"
					continue
				}
				if !entry {
					continue
				}
				k, v, _ := strings.Cut(l, "=")
				switch k {
				case "Name":
					a.Name = v
				case "NoDisplay", "Hidden":
					hidden = hidden || v == "true"
				case "Type":
					hidden = hidden || v != "Application"
				case "X-AppImage-Version", "Version":
					if k != "Version" {
						a.Version = v
					}
				}
			}
			if hidden || a.Name == "" {
				continue
			}
			if fi, err := de.Info(); err == nil {
				a.Modified = fi.ModTime().UnixMilli()
			}
			out = append(out, a)
		}
	}
	return out, nil
}

var gnomePanels = map[string]string{"privacy": "privacy", "notifications": "notifications", "network": "network", "display": "display",
	"sound": "sound", "bluetooth": "bluetooth", "accessibility": "universal-access", "storage": "privacy"}

func openSettings(env *Env, pane string) error {
	panel := gnomePanels[pane]
	if panel == "" {
		return errf(CodeUnsupported, "Linux has no settings page for %s: it is not asked for there", pane)
	}
	if firstOf(env, "gnome-control-center") != "" {
		_, err := startDetached(env, "gnome-control-center", panel)
		return err
	}
	if firstOf(env, "systemsettings") != "" {
		_, err := startDetached(env, "systemsettings")
		return err
	}
	return errf(CodeUnsupported, "no settings app found (GNOME or KDE)")
}

func thumbnail(context.Context, *Env, string, int) ([]byte, error) {
	return nil, errf(CodeUnsupported, "on Linux only pictures (png, jpeg, gif) have thumbnails")
}

func dialogMissing(env *Env) string {
	if firstOf(env, "zenity", "kdialog") == "" {
		return "dialogs need zenity or kdialog"
	}
	return ""
}

// dialog: zenity, or kdialog.
func dialog(ctx context.Context, env *Env, d Dialog) ([]string, error) {
	var argv []string
	start := d.Folder
	if d.save && d.Name != "" {
		start = filepath.Join(d.Folder, d.Name)
	}
	if firstOf(env, "zenity") != "" {
		argv = []string{"zenity", "--file-selection", "--separator=\n"}
		if d.Title != "" {
			argv = append(argv, "--title="+d.Title)
		}
		if start != "" {
			if !d.save {
				start += "/"
			}
			argv = append(argv, "--filename="+start)
		}
		switch {
		case d.save:
			argv = append(argv, "--save", "--confirm-overwrite")
		case d.Folders:
			argv = append(argv, "--directory")
		}
		if d.Multiple && !d.save {
			argv = append(argv, "--multiple")
		}
		if len(d.Types) > 0 && !d.Folders {
			var pats []string
			for _, t := range d.Types {
				pats = append(pats, "*."+strings.TrimPrefix(t, "."))
			}
			argv = append(argv, "--file-filter="+strings.Join(pats, " "))
		}
	} else if firstOf(env, "kdialog") != "" {
		if start == "" {
			start = env.Home
		}
		switch {
		case d.save:
			argv = []string{"kdialog", "--getsavefilename", start}
		case d.Folders:
			argv = []string{"kdialog", "--getexistingdirectory", start}
		default:
			argv = []string{"kdialog", "--getopenfilename", start}
			if d.Multiple {
				argv = append(argv, "--multiple", "--separate-output")
			}
		}
		if d.Title != "" {
			argv = append(argv, "--title", d.Title)
		}
	} else {
		return nil, errf(CodeUnsupported, "%s", dialogMissing(env))
	}
	out, err := runOutFor(ctx, env, 30*time.Minute, nil, nil, argv...)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) || strings.Contains(err.Error(), "exit status 1") {
			return nil, nil // (cancelled)
		}
		return nil, err
	}
	var paths []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			paths = append(paths, l)
		}
	}
	return paths, nil
}
