// Package osutil holds the few places where macOS, Linux and Windows differ:
// path comparison, "~" expansion, the user name and file permissions.
package osutil

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// Windows reports whether this build runs on Windows.
const Windows = runtime.GOOS == "windows"

// SamePath compares two cleaned paths; case-insensitively on Windows.
func SamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if Windows {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Within reports whether path is root or inside it (both should be absolute
// and cleaned; symlinks are the caller's business).
func Within(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	if SamePath(path, root) {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if len(path) < len(prefix) {
		return false
	}
	return SamePath(path[:len(prefix)-1], prefix[:len(prefix)-1]) && os.IsPathSeparator(path[len(prefix)-1])
}

// HomeRelative splits "~", "~/x" and (on Windows) "~\x" into the part after
// the home directory. ok is false for anything else.
func HomeRelative(p string) (rest string, ok bool) {
	if p == "~" {
		return "", true
	}
	if r, found := strings.CutPrefix(p, "~/"); found {
		return r, true
	}
	if Windows {
		if r, found := strings.CutPrefix(p, `~\`); found {
			return r, true
		}
	}
	return "", false
}

// ExpandHome expands a leading "~" against home.
func ExpandHome(p, home string) string {
	if rest, ok := HomeRelative(p); ok {
		if rest == "" {
			return home
		}
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	return p
}

// UserName is the current user's login name: USER (Unix), USERNAME
// (Windows), then the OS account database.
func UserName(getenv func(string) string) string {
	for _, k := range []string{"USER", "USERNAME"} {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
	}
	if u, err := user.Current(); err == nil {
		name := u.Username
		if i := strings.LastIndexAny(name, `\/`); i >= 0 { // DOMAIN\user
			name = name[i+1:]
		}
		return name
	}
	return ""
}

// UnixPermissions reports whether file mode bits mean anything here. On
// Windows access is controlled by ACLs and Go reports 0666/0444, so
// "readable by others" checks must be skipped.
const UnixPermissions = !Windows

// InCredentialDir reports whether abs (an absolute path) lies inside one of
// the credential directories (.ssh, .gnupg, .aws, .myco) anywhere in the
// path; case-insensitively on Windows.
func InCredentialDir(abs string) bool {
	for _, part := range strings.FieldsFunc(abs, func(r rune) bool { return r == '/' || r == '\\' }) {
		for _, bad := range CredentialDirs {
			if part == bad || (Windows && strings.EqualFold(part, bad)) {
				return true
			}
		}
	}
	return false
}

// CredentialDirs are never read as attachments nor used as working dirs.
var CredentialDirs = []string{".ssh", ".gnupg", ".aws", ".myco"}
