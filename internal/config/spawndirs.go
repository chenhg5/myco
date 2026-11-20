package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SpawnDirs are the folders new sessions may start in on this computer
// (spawn.dirs in machine.yaml). None written means the home folder, "~".

// EffectiveDirs is the list as it applies: ["~"] when none is written.
func EffectiveDirs(dirs []string) []string {
	if len(dirs) == 0 {
		return []string{"~"}
	}
	return dirs
}

// ExpandDir turns "~" and "~/x" into paths under home.
func ExpandDir(dir, home string) string {
	switch {
	case dir == "~":
		return home
	case strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`):
		return filepath.Join(home, dir[2:])
	}
	return dir
}

// DirWithin reports whether dir is root or inside it (both may start with ~).
func DirWithin(dir, root, home string) bool {
	d, r := filepath.Clean(ExpandDir(dir, home)), filepath.Clean(ExpandDir(root, home))
	if d == r {
		return true
	}
	rel, err := filepath.Rel(r, d)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

// CleanSpawnDir checks a folder to allow: "~", "~/x" or an absolute path,
// not the whole disk; a trailing slash is dropped.
func CleanSpawnDir(dir string) (string, error) {
	d := strings.TrimSpace(dir)
	for len(d) > 1 && (strings.HasSuffix(d, "/") || strings.HasSuffix(d, `\`)) {
		d = d[:len(d)-1]
	}
	vol := filepath.VolumeName(d)
	switch {
	case d == "":
		return "", errors.New("name a folder")
	case d == "/" || (vol != "" && strings.Trim(d[len(vol):], `/\`) == ""):
		return "", fmt.Errorf("%q is the whole disk; allow a folder in it", d)
	case d == "~" || strings.HasPrefix(d, "~/") || strings.HasPrefix(d, `~\`) || filepath.IsAbs(d) || strings.HasPrefix(d, "/"):
		return d, nil
	}
	return "", fmt.Errorf("%q: use ~/folder or a full path", d)
}

// SpawnDirsChange is what SetSpawnDirs did.
type SpawnDirsChange struct {
	Dirs    []string // the list now (as it applies)
	Changed bool
	// Covered are added folders that were already allowed (inside one);
	// Missing are removed ones that were not in the list.
	Covered []string
	Missing []string
}

// ErrLastDir: removing the last folder would put the default (all of the
// home folder) back, which is more, not less.
var ErrLastDir = errors.New("cannot remove the last allowed folder: with none, all of the home folder is allowed")

// SetSpawnDirs adds and removes folders in spawn.dirs of machine.yaml next
// to defaultPath (adds first, so a swap works), keeping every other key as it is. A folder already inside
// an allowed one is not added again; removing leaves at least one (none
// would mean all of home). home expands "~". ErrNotJoined when there is no
// machine.yaml.
func SetSpawnDirs(defaultPath, home string, add, remove []string) (SpawnDirsChange, error) {
	var out SpawnDirsChange
	path := MachinePath(defaultPath)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return out, ErrNotJoined
	}
	if err != nil {
		return out, err
	}
	doc := map[string]any{}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return out, err
	}
	spawn, _ := doc["spawn"].(map[string]any)
	if spawn == nil {
		spawn = map[string]any{}
	}
	var dirs []string
	if list, ok := spawn["dirs"].([]any); ok {
		for _, x := range list {
			if s, ok := x.(string); ok && s != "" {
				dirs = append(dirs, s)
			}
		}
	}
	written := len(dirs) > 0
	dirs = EffectiveDirs(dirs)
	for _, a := range add {
		d, err := CleanSpawnDir(a)
		if err != nil {
			return out, err
		}
		covered := false
		for _, x := range dirs {
			if DirWithin(d, x, home) {
				covered = true
			}
		}
		if covered {
			out.Covered = append(out.Covered, d)
			continue
		}
		// Folders inside the new one are covered by it now.
		kept := dirs[:0:0]
		for _, x := range dirs {
			if !DirWithin(x, d, home) {
				kept = append(kept, x)
			}
		}
		dirs = append(kept, d)
		out.Changed = true
	}
	for _, r := range remove {
		d, err := CleanSpawnDir(r)
		if err != nil {
			return out, err
		}
		i := -1
		for j, x := range dirs {
			if x == d || filepath.Clean(ExpandDir(x, home)) == filepath.Clean(ExpandDir(d, home)) {
				i = j
			}
		}
		if i < 0 {
			out.Missing = append(out.Missing, d)
			continue
		}
		if len(dirs) == 1 {
			return out, ErrLastDir
		}
		dirs = append(dirs[:i:i], dirs[i+1:]...)
		out.Changed = true
	}
	out.Dirs = dirs
	if !out.Changed {
		return out, nil
	}
	if len(dirs) == 1 && dirs[0] == "~" && !written {
		delete(spawn, "dirs")
	} else {
		spawn["dirs"] = dirs
	}
	if len(spawn) == 0 {
		delete(doc, "spawn")
	} else {
		doc["spawn"] = spawn
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return out, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return out, err
	}
	return out, os.Rename(tmp, path)
}
