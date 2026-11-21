package oscap

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// macOS: system information from sysctl, disks from getfsstat, and the
// system's own programs (open, pbcopy, osascript, screencapture, trash)
// for the rest.

func isHidden(path string, fi fs.FileInfo) bool {
	if strings.HasPrefix(fi.Name(), ".") {
		return true
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Flags&0x8000 != 0 { // UF_HIDDEN (~/Library)
		return true
	}
	return false
}

func userDirs(home string) map[string]string {
	return map[string]string{
		"desktop": filepath.Join(home, "Desktop"), "documents": filepath.Join(home, "Documents"), "downloads": filepath.Join(home, "Downloads"),
		"pictures": filepath.Join(home, "Pictures"), "music": filepath.Join(home, "Music"), "videos": filepath.Join(home, "Movies"),
		"trash": filepath.Join(home, ".Trash"), "applications": "/Applications", "library": filepath.Join(home, "Library"),
	}
}

func fillInfo(i *SystemInfo) {
	i.Name = "macOS"
	i.Version, _ = unix.Sysctl("kern.osproductversion")
	i.Model, _ = unix.Sysctl("hw.model")
	i.CPUModel, _ = unix.Sysctl("machdep.cpu.brand_string")
	i.Memory, _ = unix.SysctlUint64("hw.memsize")
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		i.Uptime = int64(time.Since(time.Unix(tv.Sec, int64(tv.Usec)*1000)).Seconds())
	}
}

func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func disks() ([]Disk, error) {
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	buf := make([]unix.Statfs_t, n+4)
	n, err = unix.Getfsstat(buf, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	var out []Disk
	for _, s := range buf[:n] {
		fsType, mount := cstr(s.Fstypename[:]), cstr(s.Mntonname[:])
		// (the system's own volumes: Preboot, VM, Data, … are "don't browse")
		if s.Flags&unix.MNT_DONTBROWSE != 0 || fsType == "devfs" || fsType == "autofs" || s.Blocks == 0 {
			continue
		}
		bs := uint64(s.Bsize)
		out = append(out, Disk{
			Mount: mount, Device: cstr(s.Mntfromname[:]), FS: fsType, Label: filepath.Base(mount),
			Total: s.Blocks * bs, Free: s.Bfree * bs, Available: s.Bavail * bs,
			Removable: strings.HasPrefix(mount, "/Volumes/") && s.Flags&unix.MNT_LOCAL != 0,
			ScanPath:  scanPathOf(mount),
			Network:   s.Flags&unix.MNT_LOCAL == 0, ReadOnly: s.Flags&unix.MNT_RDONLY != 0,
		})
	}
	if len(out) > 0 && out[0].Mount == "/" {
		out[0].Label = "Macintosh HD"
	}
	return out, nil
}

// scanPathOf: the startup disk ("/") is sealed; its files are on the Data
// volume, which is where a scan should look.
func scanPathOf(mount string) string {
	if mount == "/" {
		if _, err := os.Stat("/System/Volumes/Data/Users"); err == nil {
			return "/System/Volumes/Data"
		}
	}
	return ""
}

func isVolumeRoot(p string) bool {
	return filepath.Dir(p) == "/Volumes"
}

// trash: the system's trash command (macOS 15 and later: "Put Back" works),
// otherwise a move into the Trash of the file's disk.
func trash(ctx context.Context, env *Env, paths []string) []error {
	errs := make([]error, len(paths))
	for i, p := range paths {
		if _, err := os.Stat("/usr/bin/trash"); err == nil {
			if _, err := runOut(ctx, env, nil, nil, "/usr/bin/trash", p); err == nil {
				continue
			}
		}
		errs[i] = moveToTrash(env, p)
	}
	return errs
}

func moveToTrash(env *Env, p string) error {
	dir := filepath.Join(env.Home, ".Trash")
	if fi, err := os.Lstat(p); err == nil {
		if home, err := os.Stat(env.Home); err == nil {
			d1, _ := devOf(fi)
			d2, _ := devOf(home)
			if d1 != d2 && strings.HasPrefix(p, "/Volumes/") {
				parts := strings.SplitN(strings.TrimPrefix(p, "/Volumes/"), "/", 2)
				dir = filepath.Join("/Volumes", parts[0], ".Trashes", strconv.Itoa(os.Getuid()))
			}
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := filepath.Base(p)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 1000; i++ {
		to := filepath.Join(dir, name)
		if i > 0 {
			to = filepath.Join(dir, fmt.Sprintf("%s %s%s", stem, time.Now().Format("15.04.05"), ext))
			if i > 1 {
				to = filepath.Join(dir, fmt.Sprintf("%s %s %d%s", stem, time.Now().Format("15.04.05"), i, ext))
			}
		}
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		return os.Rename(p, to)
	}
	return errf(CodeFailed, "no free name in %s", dir)
}

func reveal(env *Env, p string) error {
	_, err := runOut(context.Background(), env, nil, nil, "/usr/bin/open", "-R", p)
	return err
}

func open(env *Env, p, app string) error {
	argv := []string{"/usr/bin/open"}
	if app != "" {
		argv = append(argv, "-a", app)
	}
	_, err := runOut(context.Background(), env, nil, nil, append(argv, p)...)
	return err
}

// (pbcopy and pbpaste read the locale: UTF-8, or Chinese becomes "?")
var utf8Env = map[string]string{"LC_CTYPE": "UTF-8", "LANG": "en_US.UTF-8"}

func clipboardMissing(*Env) string { return "" }

func clipboardGet(ctx context.Context, env *Env) (string, error) {
	out, err := runOut(ctx, env, nil, utf8Env, "/usr/bin/pbpaste")
	return string(out), err
}

func clipboardSet(ctx context.Context, env *Env, s string) error {
	_, err := runOut(ctx, env, []byte(s), utf8Env, "/usr/bin/pbcopy")
	return err
}

func notifyMissing(*Env) string { return "" }

func notify(ctx context.Context, env *Env, title, subtitle, body string, sound bool) error {
	// (the texts as arguments: no quoting inside the script)
	script := []string{"on run argv", "display notification (item 3 of argv) with title (item 1 of argv) subtitle (item 2 of argv)"}
	if sound {
		script[1] += ` sound name "default"`
	}
	script = append(script, "end run")
	argv := []string{"/usr/bin/osascript"}
	for _, l := range script {
		argv = append(argv, "-e", l)
	}
	if title == "" {
		title = "myco"
	}
	_, err := runOut(ctx, env, nil, nil, append(argv, title, subtitle, body)...)
	return err
}

func screenMissing(*Env) string { return "" }

// capture: screencapture, without the sound. Needs 屏幕录制 (Screen
// Recording) for myco in System Settings; without it macOS gives only the
// desktop picture.
func capture(ctx context.Context, env *Env, file string, display int, r *Region) (bool, error) {
	argv := []string{"/usr/sbin/screencapture", "-x", "-t", "png"}
	if display > 0 {
		argv = append(argv, "-D", strconv.Itoa(display))
	}
	if r != nil {
		argv = append(argv, "-R", fmt.Sprintf("%d,%d,%d,%d", r.X, r.Y, r.Width, r.Height))
	}
	_, err := runOut(ctx, env, nil, nil, append(argv, file)...)
	return r != nil, err
}

// usage: the CPU from top's second sample (the first counts since boot),
// memory from vm_stat as Activity Monitor counts it (apps, wired,
// compressed), the load from sysctl.
func usage(ctx context.Context, env *Env) (Usage, error) {
	u := Usage{}
	u.MemoryTotal, _ = unix.SysctlUint64("hw.memsize")
	if out, err := runOut(ctx, env, nil, nil, "/usr/bin/vm_stat"); err == nil {
		page := uint64(4096)
		pages := map[string]uint64{}
		for i, l := range strings.Split(string(out), "\n") {
			if i == 0 {
				if j := strings.Index(l, "page size of "); j >= 0 {
					if n, err := strconv.ParseUint(strings.Fields(l[j+13:])[0], 10, 64); err == nil {
						page = n
					}
				}
				continue
			}
			k, v, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), "."), 10, 64)
			if err == nil {
				pages[strings.TrimSpace(k)] = n
			}
		}
		used := pages["Anonymous pages"] - pages["Pages purgeable"] + pages["Pages wired down"] + pages["Pages occupied by compressor"]
		if pages["Anonymous pages"] == 0 {
			used = pages["Pages active"] + pages["Pages wired down"] + pages["Pages occupied by compressor"]
		}
		u.MemoryUsed = used * page
		if u.MemoryTotal > u.MemoryUsed {
			u.MemoryAvailable = u.MemoryTotal - u.MemoryUsed
		}
	}
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil && len(raw) >= 24 {
		// struct loadavg { fixpt_t ldavg[3]; long fscale; }
		le := func(b []byte) uint64 {
			var v uint64
			for i := len(b) - 1; i >= 0; i-- {
				v = v<<8 | uint64(b[i])
			}
			return v
		}
		scale := float64(le(raw[16:24]))
		if scale > 0 {
			for i := 0; i < 3; i++ {
				u.Load = append(u.Load, float64(le(raw[i*4:i*4+4]))/scale)
			}
		}
	}
	if out, err := runOut(ctx, env, nil, nil, "/usr/bin/top", "-l", "2", "-n", "0", "-s", "1"); err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			// CPU usage: 5.12% user, 8.33% sys, 86.54% idle
			if j := strings.Index(l, "% idle"); j >= 0 && strings.HasPrefix(l, "CPU usage") {
				f := strings.Fields(l[:j])
				if idle, err := strconv.ParseFloat(f[len(f)-1], 64); err == nil {
					u.CPU = 100 - idle
				}
			}
		}
	}
	return u, nil
}

// apps: the .app bundles in the usual places, read from their Info.plist.
func apps(ctx context.Context, env *Env) ([]AppInfo, error) {
	var out []AppInfo
	seen := map[string]bool{}
	for _, dir := range []string{"/Applications", "/Applications/Utilities", filepath.Join(env.Home, "Applications"), "/System/Applications", "/System/Applications/Utilities"} {
		des, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range des {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !strings.HasSuffix(de.Name(), ".app") {
				continue
			}
			p := filepath.Join(dir, de.Name())
			if seen[p] {
				continue
			}
			seen[p] = true
			a := AppInfo{Name: strings.TrimSuffix(de.Name(), ".app"), Path: p, System: strings.HasPrefix(p, "/System/")}
			if fi, err := de.Info(); err == nil {
				a.Modified = fi.ModTime().UnixMilli()
			}
			kv := plistStrings(ctx, env, filepath.Join(p, "Contents", "Info.plist"))
			if v := kv["CFBundleDisplayName"]; v != "" {
				a.Name = v
			} else if v := kv["CFBundleName"]; v != "" && !strings.Contains(v, "$(") {
				a.Name = v
			}
			a.ID = kv["CFBundleIdentifier"]
			a.Version = kv["CFBundleShortVersionString"]
			if a.Version == "" {
				a.Version = kv["CFBundleVersion"]
			}
			out = append(out, a)
		}
	}
	return out, nil
}

// plistStrings: the top-level string values of a property list (a binary
// one through plutil).
func plistStrings(ctx context.Context, env *Env, file string) map[string]string {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	if bytes.HasPrefix(b, []byte("bplist")) {
		if b, err = runOut(ctx, env, nil, nil, "/usr/bin/plutil", "-convert", "xml1", "-o", "-", file); err != nil {
			return nil
		}
	}
	return xmlPlistStrings(b)
}

var settingsURLs = map[string]string{
	"full_disk":        "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles",
	"screen_recording": "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture",
	"accessibility":    "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility",
	"privacy":          "x-apple.systempreferences:com.apple.preference.security?Privacy",
	"notifications":    "x-apple.systempreferences:com.apple.preference.notifications",
	"storage":          "x-apple.systempreferences:com.apple.settings.Storage",
	"network":          "x-apple.systempreferences:com.apple.preference.network",
	"display":          "x-apple.systempreferences:com.apple.preference.displays",
	"sound":            "x-apple.systempreferences:com.apple.preference.sound",
	"bluetooth":        "x-apple.systempreferences:com.apple.preferences.Bluetooth",
}

func openSettings(env *Env, pane string) error {
	_, err := runOut(context.Background(), env, nil, nil, "/usr/bin/open", settingsURLs[pane])
	return err
}

// thumbnail: Quick Look's (PDF, video, documents, …).
func thumbnail(ctx context.Context, env *Env, p string, size int) ([]byte, error) {
	dir, err := os.MkdirTemp("", "myco-thumb-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if _, err := runOut(ctx, env, nil, nil, "/usr/bin/qlmanage", "-t", "-s", strconv.Itoa(size), "-o", dir, p); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, filepath.Base(p)+".png"))
	if err != nil {
		return nil, errf(CodeUnsupported, "Quick Look has no picture of %s", filepath.Base(p))
	}
	return b, nil
}

func dialogMissing(*Env) string { return "" }

// dialog: AppleScript's choose file / folder / file name, in front.
func dialog(ctx context.Context, env *Env, d Dialog) ([]string, error) {
	lines := []string{"on run argv", "activate", "set p to item 1 of argv"}
	opts := ` with prompt p`
	if d.Folder != "" {
		lines = append(lines, "set f to POSIX file (item 2 of argv)")
		opts += " default location f"
	}
	switch {
	case d.save:
		lines = append(lines, "set n to item 3 of argv", "return POSIX path of (choose file name"+opts+" default name n)")
	case d.Folders:
		if d.Multiple {
			opts += " with multiple selections allowed"
			lines = append(lines, "set out to {}", "repeat with x in (choose folder"+opts+")", "set end of out to POSIX path of x", "end repeat",
				"set AppleScript's text item delimiters to linefeed", "return out as text")
		} else {
			lines = append(lines, "return POSIX path of (choose folder"+opts+")")
		}
	default:
		if len(d.Types) > 0 {
			var ts []string
			for _, t := range d.Types {
				ts = append(ts, strconv.Quote(strings.TrimPrefix(t, ".")))
			}
			opts += " of type {" + strings.Join(ts, ", ") + "}"
		}
		if d.Multiple {
			opts += " with multiple selections allowed"
			lines = append(lines, "set out to {}", "repeat with x in (choose file"+opts+")", "set end of out to POSIX path of x", "end repeat",
				"set AppleScript's text item delimiters to linefeed", "return out as text")
		} else {
			lines = append(lines, "return POSIX path of (choose file"+opts+")")
		}
	}
	lines = append(lines, "end run")
	argv := []string{"/usr/bin/osascript"}
	for _, l := range lines {
		argv = append(argv, "-e", l)
	}
	title := d.Title
	if title == "" {
		title = "选择"
	}
	out, err := runOutFor(ctx, env, 30*time.Minute, nil, nil, append(argv, title, d.Folder, d.Name)...)
	if err != nil {
		if strings.Contains(err.Error(), "-128") { // User canceled.
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			paths = append(paths, strings.TrimSuffix(l, "/"))
		}
	}
	return paths, nil
}
