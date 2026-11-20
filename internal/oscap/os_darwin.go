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
			Network:   s.Flags&unix.MNT_LOCAL == 0, ReadOnly: s.Flags&unix.MNT_RDONLY != 0,
		})
	}
	if len(out) > 0 && out[0].Mount == "/" {
		out[0].Label = "Macintosh HD"
	}
	return out, nil
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
