// Package backup makes and restores copies of a hub's data: its SQLite
// database (a consistent snapshot, taken while the hub runs) and its files.
//
// A backup is a .tar.gz laid out like the data directory it came from
// (hub.db, files/..., and for a whole server workspaces/<name>/...), with
// BACKUP.json at the end: what it holds and the sha256 of every entry.
// Restoring checks every entry against it before anything is replaced.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/myco-ai/myco/internal/infra/hubkey"
)

// Format is the version of the archive layout.
const Format = 1

// ManifestName is the archive entry that describes a backup.
const ManifestName = "BACKUP.json"

// Source is one hub to back up: its database and its files directory,
// stored under Prefix ("" for the hub itself, "workspaces/<name>" for a
// workspace of a server).
type Source struct {
	Prefix   string
	DBPath   string
	FilesDir string
}

// Manifest describes a backup.
type Manifest struct {
	Format    int               `json:"format"`
	CreatedAt time.Time         `json:"created_at"`
	Version   string            `json:"myco_version,omitempty"`
	Scope     string            `json:"scope"` // "hub", "server" or a workspace's name
	Hubs      []string          `json:"hubs"`  // the prefixes ("" = the hub itself)
	Files     int               `json:"files"`
	Bytes     int64             `json:"bytes"`
	SHA256    map[string]string `json:"sha256"`
}

// Create writes a backup of sources to w. The databases are copied with
// VACUUM INTO (consistent while the hub writes) and checked before they
// go in; files are copied as they are.
func Create(ctx context.Context, w io.Writer, sources []Source, scope, version string, now time.Time) (*Manifest, error) {
	tmp, err := os.MkdirTemp("", "myco-backup-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	m := &Manifest{Format: Format, CreatedAt: now.UTC(), Version: version, Scope: scope, SHA256: map[string]string{}}
	for i, s := range sources {
		m.Hubs = append(m.Hubs, s.Prefix)
		snap := filepath.Join(tmp, fmt.Sprintf("hub-%d.db", i))
		if err := Snapshot(ctx, s.DBPath, snap); err != nil {
			return nil, fmt.Errorf("database %s: %w", s.DBPath, err)
		}
		if err := m.add(tw, path.Join(s.Prefix, "hub.db"), snap, now); err != nil {
			return nil, err
		}
		_ = os.Remove(snap)
		// The hub's own key (i119), without which the keys it keeps for
		// members cannot be opened: the backup is a secret with it.
		if key := filepath.Join(filepath.Dir(s.DBPath), hubkey.FileName); fileExists(key) {
			if err := m.add(tw, path.Join(s.Prefix, hubkey.FileName), key, now); err != nil {
				return nil, err
			}
		}
		if s.FilesDir == "" {
			continue
		}
		err := filepath.WalkDir(s.FilesDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) && p == s.FilesDir {
					return filepath.SkipDir // no files yet
				}
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !d.Type().IsRegular() {
				return nil // directories are implied; nothing else belongs here
			}
			rel, err := filepath.Rel(s.FilesDir, p)
			if err != nil {
				return err
			}
			return m.add(tw, path.Join(s.Prefix, "files", filepath.ToSlash(rel)), p, now)
		})
		if err != nil {
			return nil, err
		}
	}
	b, _ := json.MarshalIndent(m, "", " ")
	if err := tw.WriteHeader(&tar.Header{Name: ManifestName, Mode: 0o600, Size: int64(len(b)), ModTime: now, Typeflag: tar.TypeReg}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(b); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return m, gz.Close()
}

func (m *Manifest) add(tw *tar.Writer, name, src string, now time.Time) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: st.Size(), ModTime: st.ModTime(), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tw, h), io.LimitReader(f, st.Size()))
	if err != nil {
		return err
	}
	if n != st.Size() {
		return fmt.Errorf("%s changed while it was copied", src)
	}
	m.SHA256[name] = hex.EncodeToString(h.Sum(nil))
	m.Files++
	m.Bytes += n
	return nil
}

// Snapshot copies the SQLite database at src to dst (which must not
// exist) as one consistent snapshot, and checks the copy.
func Snapshot(ctx context.Context, src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+src+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		return err
	}
	return Check(ctx, dst)
}

// Check opens a database file and runs SQLite's integrity check.
func Check(ctx context.Context, file string) error {
	db, err := sql.Open("sqlite", "file:"+file+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var res string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("integrity check: %s", res)
	}
	return nil
}

// Extract unpacks a backup into dir (empty or new) and checks every entry
// against the manifest, and each database's integrity. Nothing outside
// dir is written.
func Extract(ctx context.Context, r io.Reader, dir string) (*Manifest, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("not a myco backup: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	got := map[string]string{}
	var m *Manifest
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading the backup: %w", err)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		name := h.Name
		if h.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("unexpected entry %q in the backup", name)
		}
		if name == ManifestName {
			var mm Manifest
			if err := json.NewDecoder(io.LimitReader(tr, 64<<20)).Decode(&mm); err != nil {
				return nil, fmt.Errorf("reading the backup's manifest: %w", err)
			}
			m = &mm
			continue
		}
		clean := path.Clean(name)
		if clean != name || path.IsAbs(name) || strings.HasPrefix(name, "../") || name == ".." || strings.Contains(name, "\\") {
			return nil, fmt.Errorf("unsafe path %q in the backup", name)
		}
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		hs := sha256.New()
		_, err = io.Copy(io.MultiWriter(f, hs), tr)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		got[name] = hex.EncodeToString(hs.Sum(nil))
	}
	if m == nil {
		return nil, errors.New("not a myco backup: it has no " + ManifestName)
	}
	if m.Format != Format {
		return nil, fmt.Errorf("backup format %d is not supported by this myco (it reads %d): upgrade myco", m.Format, Format)
	}
	if len(got) != len(m.SHA256) {
		return nil, fmt.Errorf("the backup is incomplete: %d of %d entries", len(got), len(m.SHA256))
	}
	for name, sum := range m.SHA256 {
		if got[name] != sum {
			return nil, fmt.Errorf("the backup is damaged: %s does not match its checksum", name)
		}
	}
	for _, p := range m.Hubs {
		if err := Check(ctx, filepath.Join(dir, filepath.FromSlash(path.Join(p, "hub.db")))); err != nil {
			return nil, fmt.Errorf("database %s in the backup: %w", path.Join(p, "hub.db"), err)
		}
	}
	return m, nil
}

// Info is a backup kept on disk.
type Info struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// Suffix ends every backup's file name.
const Suffix = ".tar.gz"

// List lists the backups in dir, newest first.
func List(dir string) ([]Info, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Info{}
	for _, e := range ents {
		if !e.Type().IsRegular() || !strings.HasPrefix(e.Name(), "myco-backup-") || !strings.HasSuffix(e.Name(), Suffix) {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{Name: e.Name(), Size: st.Size(), CreatedAt: st.ModTime().UTC()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ValidName reports whether name can be a backup in a backups directory
// (no path, the expected shape).
func ValidName(name string) bool {
	return strings.HasPrefix(name, "myco-backup-") && strings.HasSuffix(name, Suffix) &&
		!strings.ContainsAny(name, "/\\") && !strings.Contains(name, "..")
}

// FileName is the name of a backup of scope made at t.
func FileName(scope string, t time.Time) string {
	return "myco-backup-" + scope + "-" + t.UTC().Format("20060102-150405") + Suffix
}

// Write makes a backup of sources into dir (as FileName(scope, now)),
// through a temporary file, so a half-written backup never looks done.
func Write(ctx context.Context, dir string, sources []Source, scope, version string, now time.Time) (Info, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Info{}, err
	}
	name := FileName(scope, now)
	tmp, err := os.CreateTemp(dir, ".partial-")
	if err != nil {
		return Info{}, err
	}
	defer os.Remove(tmp.Name())
	if _, err := Create(ctx, tmp, sources, scope, version, now); err != nil {
		tmp.Close()
		return Info{}, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return Info{}, err
	}
	if err := tmp.Close(); err != nil {
		return Info{}, err
	}
	dst := filepath.Join(dir, name)
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return Info{}, err
	}
	_ = os.Chmod(dst, 0o600)
	st, err := os.Stat(dst)
	if err != nil {
		return Info{}, err
	}
	return Info{Name: name, Size: st.Size(), CreatedAt: st.ModTime().UTC()}, nil
}

// Prune keeps the newest keep backups in dir and removes the others.
func Prune(dir string, keep int) error {
	list, err := List(dir)
	if err != nil || keep <= 0 {
		return err
	}
	for i, b := range list {
		if i >= keep {
			if err := os.Remove(filepath.Join(dir, b.Name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// Restore puts the hubs of a backup, extracted (by Extract) into
// extracted, in place under dataDir. What each one had (hub.db with its
// -wal and -shm files, and files/) is moved aside to
// <its dir>/before-restore-<time>/, never deleted. The hub must not be
// running. extracted should be inside dataDir (same file system).
func Restore(extracted, dataDir string, m *Manifest, now time.Time) (aside []string, err error) {
	stamp := "before-restore-" + now.UTC().Format("20060102-150405")
	for _, p := range m.Hubs {
		src := filepath.Join(extracted, filepath.FromSlash(p))
		dst := filepath.Join(dataDir, filepath.FromSlash(p))
		if err := os.MkdirAll(dst, 0o700); err != nil {
			return aside, err
		}
		keep := filepath.Join(dst, stamp)
		moved := false
		for _, n := range []string{"hub.db", "hub.db-wal", "hub.db-shm", "files"} {
			if _, err := os.Lstat(filepath.Join(dst, n)); err != nil {
				continue
			}
			if !moved {
				if err := os.MkdirAll(keep, 0o700); err != nil {
					return aside, err
				}
				aside, moved = append(aside, keep), true
			}
			if err := os.Rename(filepath.Join(dst, n), filepath.Join(keep, n)); err != nil {
				return aside, err
			}
		}
		if err := os.Rename(filepath.Join(src, "hub.db"), filepath.Join(dst, "hub.db")); err != nil {
			return aside, err
		}
		if _, err := os.Stat(filepath.Join(src, "files")); err == nil {
			if err := os.Rename(filepath.Join(src, "files"), filepath.Join(dst, "files")); err != nil {
				return aside, err
			}
		}
		// The hub's key comes back with the database it opens (one from a
		// backup without it is left as it is).
		if key := filepath.Join(src, hubkey.FileName); fileExists(key) {
			cur := filepath.Join(dst, hubkey.FileName)
			if fileExists(cur) {
				if !moved {
					if err := os.MkdirAll(keep, 0o700); err != nil {
						return aside, err
					}
					aside, moved = append(aside, keep), true
				}
				if err := os.Rename(cur, filepath.Join(keep, hubkey.FileName)); err != nil {
					return aside, err
				}
			}
			if err := os.Rename(key, cur); err != nil {
				return aside, err
			}
			_ = os.Chmod(cur, 0o600)
		}
	}
	return aside, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// Sources lists the hubs of a server whose main database is dbPath: the
// hub itself and, with workspaces, each workspace under
// <dir>/workspaces/<name>/.
func Sources(dbPath, filesDir string, workspaces bool) []Source {
	if filesDir == "" {
		filesDir = filepath.Join(filepath.Dir(dbPath), "files")
	}
	out := []Source{{DBPath: dbPath, FilesDir: filesDir}}
	if !workspaces {
		return out
	}
	dir := filepath.Join(filepath.Dir(dbPath), "workspaces")
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		db := filepath.Join(dir, e.Name(), "hub.db")
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(db); err != nil {
			continue
		}
		out = append(out, Source{Prefix: "workspaces/" + e.Name(), DBPath: db, FilesDir: filepath.Join(dir, e.Name(), "files")})
	}
	return out
}
