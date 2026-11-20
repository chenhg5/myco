package oscap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

func init() {
	register(
		&Cap{Name: "fs.paths", About: "常用的文件夹：个人文件夹、桌面、文稿、下载、图片、临时、缓存、废纸篓", Fn: fsPaths},
		&Cap{Name: "fs.list", About: "列一个文件夹", Fn: fsList},
		&Cap{Name: "fs.stat", About: "一个文件或文件夹的大小、时间、类型", Fn: fsStat},
		&Cap{Name: "fs.read", About: "读文件（文字或 base64，可按范围读）", Fn: fsRead},
		&Cap{Name: "fs.write", About: "写文件（覆盖或追加）", Fn: fsWrite},
		&Cap{Name: "fs.mkdir", About: "新建文件夹（连同上层）", Fn: fsMkdir},
		&Cap{Name: "fs.move", About: "移动或改名", Fn: fsMove},
		&Cap{Name: "fs.copy", About: "复制文件或整个文件夹", Fn: fsCopy},
		&Cap{Name: "fs.delete", About: "永久删除", Fn: fsDelete},
		&Cap{Name: "fs.trash", About: "移进废纸篓 / 回收站", Fn: fsTrash},
		&Cap{Name: "fs.scan", Job: true, About: "按大小汇总文件夹，找最大的文件（带进度）", Fn: fsScan},
		&Cap{Name: "fs.reveal", About: "在访达 / 资源管理器里显示", Fn: fsReveal},
		&Cap{Name: "fs.open", About: "用默认的程序打开（Mac 上可以指定程序）", Fn: fsOpen},
	)
}

// abs expands "~" and requires an absolute path.
func (e *Env) abs(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errf(CodeInvalid, "a path is needed")
	}
	if p == "~" {
		p = e.Home
	} else if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		p = filepath.Join(e.Home, p[2:])
	}
	if !filepath.IsAbs(p) {
		return "", errf(CodeInvalid, "%q is not an absolute path (or ~/…)", p)
	}
	return filepath.Clean(p), nil
}

// Entry is a file or folder.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// file, dir, link or other
	Type string `json:"type"`
	Size int64  `json:"size"`
	// Modified, in milliseconds since 1970.
	Modified int64  `json:"modified"`
	Mode     string `json:"mode"`
	Hidden   bool   `json:"hidden,omitempty"`
	// Target: where a link points.
	Target string `json:"target,omitempty"`
}

func entryOf(path string, fi fs.FileInfo) Entry {
	e := Entry{Name: fi.Name(), Path: path, Size: fi.Size(), Modified: fi.ModTime().UnixMilli(), Mode: fi.Mode().String(), Hidden: isHidden(path, fi)}
	switch m := fi.Mode(); {
	case m.IsDir():
		e.Type = "dir"
	case m&fs.ModeSymlink != 0:
		e.Type = "link"
		e.Target, _ = os.Readlink(path)
	case m.IsRegular():
		e.Type = "file"
	default:
		e.Type = "other"
	}
	if e.Type == "dir" {
		e.Size = 0
	}
	return e
}

func fsPaths(_ context.Context, env *Env, _ json.RawMessage, _ Emit) (any, error) {
	out := map[string]string{"home": env.Home, "temp": os.TempDir()}
	if d, err := os.UserCacheDir(); err == nil {
		out["cache"] = d
	}
	if d, err := os.UserConfigDir(); err == nil {
		out["config"] = d
	}
	for k, v := range userDirs(env.Home) {
		out[k] = v
	}
	return out, nil
}

func fsList(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path   string `json:"path"`
		Hidden bool   `json:"hidden"`
		Limit  int    `json:"limit"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	dir, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	if in.Limit <= 0 || in.Limit > 50000 {
		in.Limit = 10000
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := struct {
		Path      string  `json:"path"`
		Entries   []Entry `json:"entries"`
		Truncated bool    `json:"truncated,omitempty"`
	}{Path: dir, Entries: []Entry{}}
	for _, de := range des {
		fi, err := de.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(dir, de.Name())
		e := entryOf(p, fi)
		if e.Hidden && !in.Hidden {
			continue
		}
		if len(out.Entries) >= in.Limit {
			out.Truncated = true
			break
		}
		out.Entries = append(out.Entries, e)
	}
	return out, nil
}

func fsStat(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	return entryOf(p, fi), nil
}

// The most fs.read gives at once (a page reads a larger file in parts).
const maxRead = 32 << 20

func fsRead(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path     string `json:"path"`
		Encoding string `json:"encoding"` // "utf8" (default) or "base64"
		Offset   int64  `json:"offset"`
		Length   int64  `json:"length"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, errf(CodeInvalid, "%s is a folder: list it (fs.list)", p)
	}
	if in.Length <= 0 {
		in.Length = fi.Size() - in.Offset
	}
	if in.Length > maxRead {
		if in.Offset == 0 && fi.Size() > maxRead && in.Length == fi.Size() {
			return nil, errf(CodeTooLarge, "%s is %d MB: read it in parts (offset, length; at most %d MB each)", p, fi.Size()>>20, maxRead>>20)
		}
		in.Length = maxRead
	}
	if in.Length < 0 {
		in.Length = 0
	}
	buf := make([]byte, in.Length)
	n, err := f.ReadAt(buf, in.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	buf = buf[:n]
	enc := in.Encoding
	if enc != "base64" && !utf8.Valid(buf) {
		enc = "base64"
	}
	out := map[string]any{"path": p, "size": fi.Size(), "offset": in.Offset, "length": n, "eof": in.Offset+int64(n) >= fi.Size()}
	if enc == "base64" {
		out["encoding"], out["content"] = "base64", base64.StdEncoding.EncodeToString(buf)
	} else {
		out["encoding"], out["content"] = "utf8", string(buf)
	}
	return out, nil
}

func content(s, enc string) ([]byte, error) {
	if enc == "base64" {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, errf(CodeInvalid, "content is not base64: %v", err)
		}
		return b, nil
	}
	return []byte(s), nil
}

func fsWrite(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	in := struct {
		Path     string `json:"path"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		Append   bool   `json:"append"`
		// Mkdirs: make the folders above it (default true).
		Mkdirs *bool `json:"mkdirs"`
		// Exclusive: fail (code "exists") if it is there already.
		Exclusive bool `json:"exclusive"`
	}{}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	b, err := content(in.Content, in.Encoding)
	if err != nil {
		return nil, err
	}
	if in.Mkdirs == nil || *in.Mkdirs {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
	}
	flag := os.O_WRONLY | os.O_CREATE
	switch {
	case in.Append:
		flag |= os.O_APPEND
	case in.Exclusive:
		flag |= os.O_EXCL
	default:
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(p, flag, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	return entryOf(p, fi), nil
}

func fsMkdir(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return nil, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	return entryOf(p, fi), nil
}

type fromTo struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Overwrite bool   `json:"overwrite"`
}

func (env *Env) fromTo(args json.RawMessage) (string, string, bool, error) {
	var in fromTo
	if err := decode(args, &in); err != nil {
		return "", "", false, err
	}
	from, err := env.abs(in.From)
	if err != nil {
		return "", "", false, err
	}
	to, err := env.abs(in.To)
	if err != nil {
		return "", "", false, err
	}
	if _, err := os.Lstat(from); err != nil {
		return "", "", false, err
	}
	if !in.Overwrite {
		if _, err := os.Lstat(to); err == nil {
			return "", "", false, errf(CodeExists, "%s is there already (overwrite: true replaces it)", to)
		}
	}
	if from == to || strings.HasPrefix(to, from+string(filepath.Separator)) {
		return "", "", false, errf(CodeInvalid, "cannot put %s inside itself", from)
	}
	return from, to, in.Overwrite, nil
}

func fsMove(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	from, to, over, err := env.fromTo(args)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return nil, err
	}
	if over {
		if fi, err := os.Lstat(to); err == nil && fi.IsDir() {
			if err := os.RemoveAll(to); err != nil {
				return nil, err
			}
		}
	}
	if err := os.Rename(from, to); err != nil {
		var le *os.LinkError
		if !errors.As(err, &le) || !errors.Is(le.Err, syscall.EXDEV) && !crossDevice(le.Err) {
			return nil, err
		}
		// another disk: copy, then remove
		if err := copyAll(from, to); err != nil {
			return nil, err
		}
		if err := os.RemoveAll(from); err != nil {
			return nil, err
		}
	}
	fi, err := os.Lstat(to)
	if err != nil {
		return nil, err
	}
	return entryOf(to, fi), nil
}

func fsCopy(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	from, to, over, err := env.fromTo(args)
	if err != nil {
		return nil, err
	}
	if over {
		if err := os.RemoveAll(to); err != nil {
			return nil, err
		}
	}
	if err := copyAll(from, to); err != nil {
		return nil, err
	}
	fi, err := os.Lstat(to)
	if err != nil {
		return nil, err
	}
	return entryOf(to, fi), nil
}

// copyAll copies a file, a link or a whole folder.
func copyAll(from, to string) error {
	fi, err := os.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		t, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(t, to)
	case fi.IsDir():
		if err := os.MkdirAll(to, fi.Mode().Perm()|0o700); err != nil {
			return err
		}
		des, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, de := range des {
			if err := copyAll(filepath.Join(from, de.Name()), filepath.Join(to, de.Name())); err != nil {
				return err
			}
		}
		return nil
	case fi.Mode().IsRegular():
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		src, err := os.Open(from)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			return err
		}
		if err := dst.Close(); err != nil {
			return err
		}
		return os.Chtimes(to, fi.ModTime(), fi.ModTime())
	}
	return errf(CodeInvalid, "%s is not a file, folder or link", from)
}

// tooBroad: a path no single call should delete or trash whole (a slip in
// a page's code, not a rule about what apps may do).
func (env *Env) tooBroad(p string) bool {
	if p == env.Home || filepath.Dir(p) == p {
		return true
	}
	return isVolumeRoot(p)
}

func fsDelete(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path      string   `json:"path"`
		Paths     []string `json:"paths"`
		Recursive bool     `json:"recursive"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	paths := in.Paths
	if in.Path != "" {
		paths = append([]string{in.Path}, paths...)
	}
	if len(paths) == 0 {
		return nil, errf(CodeInvalid, "a path is needed")
	}
	res := batchResult{Done: []string{}, Failed: []failed{}}
	for _, raw := range paths {
		p, err := env.abs(raw)
		if err == nil && env.tooBroad(p) {
			err = errf(CodeInvalid, "%s is a whole disk or the home folder", p)
		}
		if err == nil {
			if in.Recursive {
				if _, err = os.Lstat(p); err == nil {
					err = os.RemoveAll(p)
				}
			} else {
				err = os.Remove(p)
			}
		}
		res.add(raw, p, err)
	}
	return res, nil
}

type failed struct {
	Path  string `json:"path"`
	Error *Error `json:"error"`
}

// batchResult: what a call on several paths did with each.
type batchResult struct {
	Done   []string `json:"done"`
	Failed []failed `json:"failed"`
}

func (r *batchResult) add(raw, p string, err error) {
	if err != nil {
		if p == "" {
			p = raw
		}
		r.Failed = append(r.Failed, failed{Path: p, Error: AsError(err)})
		return
	}
	r.Done = append(r.Done, p)
}

func fsTrash(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	paths := in.Paths
	if in.Path != "" {
		paths = append([]string{in.Path}, paths...)
	}
	if len(paths) == 0 {
		return nil, errf(CodeInvalid, "a path is needed")
	}
	res := batchResult{Done: []string{}, Failed: []failed{}}
	var ok []string
	for _, raw := range paths {
		p, err := env.abs(raw)
		if err == nil && env.tooBroad(p) {
			err = errf(CodeInvalid, "%s is a whole disk or the home folder", p)
		}
		if err == nil {
			_, err = os.Lstat(p)
		}
		if err != nil {
			res.add(raw, p, err)
			continue
		}
		ok = append(ok, p)
	}
	if len(ok) > 0 {
		errs := trash(ctx, env, ok)
		for i, p := range ok {
			res.add(p, p, errs[i])
		}
	}
	return res, nil
}

func fsReveal(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(p); err != nil {
		return nil, err
	}
	return true, reveal(env, p)
}

func fsOpen(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path string `json:"path"`
		// App: the program to open it with (macOS: a name, "Visual Studio Code", or a .app).
		App string `json:"app"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p); err != nil {
		return nil, err
	}
	return true, open(env, p, in.App)
}

// --- fs.scan ---

// Node is a folder in a scan: its total and, to the depth asked, the
// folders and files in it, largest first.
type Node struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
	// Size: the bytes of its files; Disk: what they take on the disk (blocks).
	Size     int64   `json:"size"`
	Disk     int64   `json:"disk"`
	Files    int64   `json:"files"`
	Modified int64   `json:"modified,omitempty"`
	Children []*Node `json:"children,omitempty"`
	// More: how many smaller ones are left out of Children.
	More int `json:"more,omitempty"`
}

// BigFile is one of the largest files found.
type BigFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

type scanner struct {
	ctx      context.Context
	emit     Emit
	depth    int
	top      int
	minSize  int64
	oneFS    bool
	perLevel int
	files    int64
	dirs     int64
	bytes    int64
	skipped  []string
	nskipped int
	largest  []BigFile
	seen     map[[2]uint64]bool
	last     time.Time
	current  string
}

func fsScan(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error) {
	in := struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
		// Depth: how many levels of folders to give (default 2, at most 8).
		Depth int `json:"depth"`
		// Top: how many of the largest files to give (default 50, at most 2000).
		Top int `json:"top"`
		// MinSize: files smaller than this are not in Top.
		MinSize int64 `json:"min_size"`
		// PerLevel: at most this many children per folder (default 100).
		PerLevel int `json:"per_level"`
		// OneFilesystem: do not go into other disks mounted inside (default true).
		OneFilesystem *bool `json:"one_filesystem"`
	}{}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	paths := in.Paths
	if in.Path != "" {
		paths = append([]string{in.Path}, paths...)
	}
	if len(paths) == 0 {
		return nil, errf(CodeInvalid, "a path is needed")
	}
	s := &scanner{ctx: ctx, emit: emit, depth: clamp(in.Depth, 2, 0, 8), top: clamp(in.Top, 50, 0, 2000), minSize: in.MinSize,
		oneFS: in.OneFilesystem == nil || *in.OneFilesystem, perLevel: clamp(in.PerLevel, 100, 1, 5000), seen: map[[2]uint64]bool{}}
	start := time.Now()
	roots := []*Node{}
	for _, raw := range paths {
		p, err := env.abs(raw)
		if err != nil {
			return nil, err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		dev, _ := devOf(fi)
		n := s.walk(p, fi, dev, 0)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		roots = append(roots, n)
	}
	s.progress(true)
	if s.skipped == nil {
		s.skipped = []string{}
	}
	if s.largest == nil {
		s.largest = []BigFile{}
	}
	return map[string]any{
		"roots": roots, "largest": s.largest, "files": s.files, "dirs": s.dirs, "bytes": s.bytes,
		"skipped": s.nskipped, "skipped_paths": s.skipped, "ms": time.Since(start).Milliseconds(),
	}, nil
}

func clamp(v, def, lo, hi int) int {
	if v == 0 {
		v = def
	}
	if v < lo {
		v = lo
	}
	if v > hi {
		v = hi
	}
	return v
}

func (s *scanner) progress(force bool) {
	if !force && time.Since(s.last) < 250*time.Millisecond {
		return
	}
	s.last = time.Now()
	s.emit("progress", map[string]any{"files": s.files, "dirs": s.dirs, "bytes": s.bytes, "current": s.current})
}

func (s *scanner) skip(p string) {
	s.nskipped++
	if len(s.skipped) < 100 {
		s.skipped = append(s.skipped, p)
	}
}

func (s *scanner) addLargest(p string, fi fs.FileInfo) {
	if s.top == 0 || fi.Size() < s.minSize {
		return
	}
	if len(s.largest) == s.top && fi.Size() <= s.largest[len(s.largest)-1].Size {
		return
	}
	f := BigFile{Path: p, Size: fi.Size(), Modified: fi.ModTime().UnixMilli()}
	i := sort.Search(len(s.largest), func(i int) bool { return s.largest[i].Size < f.Size })
	s.largest = append(s.largest, BigFile{})
	copy(s.largest[i+1:], s.largest[i:])
	s.largest[i] = f
	if len(s.largest) > s.top {
		s.largest = s.largest[:s.top]
	}
}

func (s *scanner) walk(p string, fi fs.FileInfo, dev uint64, level int) *Node {
	n := &Node{Path: p, Name: fi.Name(), Dir: fi.IsDir(), Modified: fi.ModTime().UnixMilli()}
	if !fi.IsDir() {
		n.Size, n.Disk = fi.Size(), diskOf(fi)
		if fi.Mode().IsRegular() {
			n.Files = 1
			if id, ok := inodeOf(fi); ok && linksOf(fi) > 1 {
				if s.seen[id] {
					n.Disk = 0 // (a hard link to a file already counted)
				}
				s.seen[id] = true
			}
			s.files++
			s.bytes += n.Size
			s.addLargest(p, fi)
		}
		return n
	}
	s.dirs++
	s.current = p
	s.progress(false)
	des, err := os.ReadDir(p)
	if err != nil {
		s.skip(p)
	}
	var kids []*Node
	for _, de := range des {
		if s.ctx.Err() != nil {
			return n
		}
		cp := filepath.Join(p, de.Name())
		cfi, err := de.Info()
		if err != nil {
			s.skip(cp)
			continue
		}
		if cfi.IsDir() && s.oneFS {
			if d, ok := devOf(cfi); ok && d != dev {
				continue
			}
		}
		k := s.walk(cp, cfi, dev, level+1)
		n.Size += k.Size
		n.Disk += k.Disk
		n.Files += k.Files
		if level < s.depth {
			if !k.Dir || level+1 >= s.depth {
				k.Children, k.More = nil, 0
			}
			kids = append(kids, k)
		}
	}
	if level < s.depth && len(kids) > 0 {
		sort.Slice(kids, func(a, b int) bool { return kids[a].Disk > kids[b].Disk })
		if len(kids) > s.perLevel {
			n.More = len(kids) - s.perLevel
			kids = kids[:s.perLevel]
		}
		n.Children = kids
	}
	return n
}
