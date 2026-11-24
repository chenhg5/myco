package oscap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/gif" // (decoders for fs.thumbnail)
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

func init() {
	register(
		&Cap{Name: "system.usage", About: "此刻的 CPU、内存（和负载）", Fn: systemUsage},
		&Cap{Name: "system.apps", About: "装了哪些应用：名字、版本、位置（可选大小）", Fn: systemApps},
		&Cap{Name: "system.settings", About: "打开系统设置的某一页（完全磁盘访问、屏幕录制、储存空间…）", Fn: systemSettings},
		&Cap{Name: "fs.search", Job: true, About: "按名字、类型、大小、时间找文件（边找边推）", Fn: fsSearch},
		&Cap{Name: "fs.watch", Job: true, About: "盯着一个文件夹，有文件新建、改动、删除就推过来", Fn: fsWatch},
		&Cap{Name: "fs.thumbnail", About: "文件的缩略图（图片都行；Mac 上 PDF、视频、文档也行）", Fn: fsThumbnail},
		&Cap{Name: "fs.pick", About: "系统的选文件 / 选文件夹框", Missing: dialogMissing, Fn: fsPick},
		&Cap{Name: "fs.saveAs", About: "系统的「存储为」框：选个位置和名字", Missing: dialogMissing, Fn: fsSaveAs},
	)
}

// --- system.usage ---

// Usage is the computer's load now.
type Usage struct {
	// CPU: percent of all cores busy (0–100), over a short sample.
	CPU float64 `json:"cpu"`
	// Memory: bytes in all, used (by apps, the system and compressed) and still available.
	MemoryTotal     uint64 `json:"memory_total"`
	MemoryUsed      uint64 `json:"memory_used"`
	MemoryAvailable uint64 `json:"memory_available"`
	// Load: the 1, 5 and 15 minute load averages (not on Windows).
	Load []float64 `json:"load,omitempty"`
}

func systemUsage(ctx context.Context, env *Env, _ json.RawMessage, _ Emit) (any, error) {
	return usage(ctx, env)
}

// --- system.apps ---

// AppInfo is an installed application.
type AppInfo struct {
	Name    string `json:"name"`
	ID      string `json:"id,omitempty"`
	Version string `json:"version,omitempty"`
	// Path: the .app (macOS), its folder (Windows, when known) or the .desktop file (Linux).
	Path      string `json:"path,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	// Size: bytes on the disk (with size: true; Windows: what the installer says).
	Size     int64 `json:"size,omitempty"`
	Modified int64 `json:"modified,omitempty"`
	// System: comes with the system (macOS /System/Applications).
	System bool `json:"system,omitempty"`
}

func systemApps(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		// Size: add up each app's files (takes a few seconds).
		Size bool `json:"size"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	as, err := apps(ctx, env)
	if err != nil {
		return nil, err
	}
	if in.Size {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		for i := range as {
			if as[i].Size > 0 || as[i].Path == "" || strings.HasSuffix(as[i].Path, ".desktop") {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(a *AppInfo) {
				defer func() { <-sem; wg.Done() }()
				a.Size = dirSize(ctx, a.Path)
			}(&as[i])
		}
		wg.Wait()
	}
	sort.Slice(as, func(a, b int) bool { return strings.ToLower(as[a].Name) < strings.ToLower(as[b].Name) })
	if as == nil {
		as = []AppInfo{}
	}
	return as, nil
}

// dirSize is what a folder's files take on the disk.
func dirSize(ctx context.Context, root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				total += diskOf(path, fi)
			}
		}
		return nil
	})
	return total
}

// --- system.settings ---

// The settings pages there are (each system opens its nearest one).
var settingsPanes = []string{"full_disk", "screen_recording", "accessibility", "notifications", "privacy", "storage", "network", "display", "sound", "bluetooth"}

func systemSettings(_ context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Pane string `json:"pane"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	ok := false
	for _, p := range settingsPanes {
		ok = ok || p == in.Pane
	}
	if !ok {
		return nil, errf(CodeInvalid, "pane is one of %s", strings.Join(settingsPanes, ", "))
	}
	return true, openSettings(env, in.Pane)
}

// --- fs.search ---

func fsSearch(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error) {
	in := struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
		// Name: a part of the name (any case), or a pattern with * and ? (report*.pdf).
		Name string `json:"name"`
		// Ext: these endings only (["jpg", "png"]).
		Ext []string `json:"ext"`
		// Type: "file" (default), "dir" or "any".
		Type    string `json:"type"`
		MinSize int64  `json:"min_size"`
		MaxSize int64  `json:"max_size"`
		// ModifiedAfter / Before: milliseconds since 1970.
		ModifiedAfter  int64 `json:"modified_after"`
		ModifiedBefore int64 `json:"modified_before"`
		// Hidden: also look in hidden files and folders (default false).
		Hidden bool `json:"hidden"`
		// Limit: stop after this many (default 500, at most 20000).
		Limit int `json:"limit"`
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
	limit := clamp(in.Limit, 500, 1, 20000)
	name := strings.ToLower(in.Name)
	pattern := strings.ContainsAny(name, "*?[")
	exts := map[string]bool{}
	for _, e := range in.Ext {
		exts["."+strings.TrimPrefix(strings.ToLower(e), ".")] = true
	}
	matches := []Entry{}
	scanned := 0
	last := time.Now()
	var batch []Entry
	flush := func() {
		if len(batch) > 0 {
			emit("match", batch)
			batch = nil
		}
		emit("progress", map[string]int{"scanned": scanned, "found": len(matches)})
		last = time.Now()
	}
	errDone := errf(CodeTooLarge, "")
	for _, raw := range paths {
		root, err := env.abs(raw)
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return nil // (no permission: go on)
			}
			scanned++
			if p != root && !in.Hidden && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if time.Since(last) > 300*time.Millisecond {
				flush()
			}
			if p == root {
				return nil
			}
			switch in.Type {
			case "dir":
				if !d.IsDir() {
					return nil
				}
			case "any":
			default:
				if d.IsDir() {
					return nil
				}
			}
			n := strings.ToLower(d.Name())
			if name != "" {
				if pattern {
					if ok, _ := filepath.Match(name, n); !ok {
						return nil
					}
				} else if !strings.Contains(n, name) {
					return nil
				}
			}
			if len(exts) > 0 && !exts[filepath.Ext(n)] {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			if in.MinSize > 0 && fi.Size() < in.MinSize || in.MaxSize > 0 && fi.Size() > in.MaxSize {
				return nil
			}
			m := fi.ModTime().UnixMilli()
			if in.ModifiedAfter > 0 && m < in.ModifiedAfter || in.ModifiedBefore > 0 && m > in.ModifiedBefore {
				return nil
			}
			e := entryOf(p, fi)
			matches = append(matches, e)
			batch = append(batch, e)
			if len(matches) >= limit {
				return errDone
			}
			return nil
		})
		if err == errDone {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	flush()
	return map[string]any{"matches": matches, "scanned": scanned, "truncated": len(matches) >= limit}, nil
}

// --- fs.watch ---

type seenFile struct {
	size int64
	mod  time.Time
	dir  bool
}

func snapshot(ctx context.Context, root string, recursive bool, max int) (map[string]seenFile, bool) {
	out := map[string]seenFile{}
	full := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || ctx.Err() != nil || p == root {
			return nil
		}
		if len(out) >= max {
			full = true
			return filepath.SkipAll
		}
		if fi, err := d.Info(); err == nil {
			out[p] = seenFile{size: fi.Size(), mod: fi.ModTime(), dir: d.IsDir()}
		}
		if d.IsDir() && !recursive {
			return filepath.SkipDir
		}
		return nil
	})
	return out, full
}

// fsWatch looks at the folder every interval and sends what changed. (A
// look, not the system's file events: the same on every system, and
// enough for a folder of up to tens of thousands of files.)
func fsWatch(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error) {
	in := struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
		// IntervalMs: how often to look (default 1000, at least 250).
		IntervalMs int `json:"interval_ms"`
		// MaxEntries: the most files it keeps track of (default 50000).
		MaxEntries int `json:"max_entries"`
	}{}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	root, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(root); err != nil {
		return nil, err
	} else if !fi.IsDir() {
		return nil, errf(CodeInvalid, "%s is not a folder", root)
	}
	every := time.Duration(clamp(in.IntervalMs, 1000, 250, 600000)) * time.Millisecond
	max := clamp(in.MaxEntries, 50000, 1, 500000)
	before, full := snapshot(ctx, root, in.Recursive, max)
	emit("ready", map[string]any{"entries": len(before), "full": full})
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return map[string]any{"stopped": true}, nil
		case <-t.C:
		}
		if _, err := os.Stat(root); err != nil {
			emit("change", []map[string]any{{"type": "removed", "path": root}})
			return map[string]any{"stopped": true, "removed": true}, nil
		}
		now, _ := snapshot(ctx, root, in.Recursive, max)
		var changes []map[string]any
		for p, f := range now {
			old, ok := before[p]
			switch {
			case !ok:
				changes = append(changes, map[string]any{"type": "created", "path": p, "dir": f.dir, "size": f.size})
			case !f.dir && (old.size != f.size || !old.mod.Equal(f.mod)):
				changes = append(changes, map[string]any{"type": "modified", "path": p, "size": f.size})
			}
		}
		for p, f := range before {
			if _, ok := now[p]; !ok {
				changes = append(changes, map[string]any{"type": "removed", "path": p, "dir": f.dir})
			}
		}
		if len(changes) > 0 {
			sort.Slice(changes, func(a, b int) bool { return changes[a]["path"].(string) < changes[b]["path"].(string) })
			emit("change", changes)
		}
		before = now
	}
}

// --- fs.thumbnail ---

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true}

func fsThumbnail(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Path string `json:"path"`
		// Size: the longer side, in pixels (default 256, at most 1024).
		Size   int    `json:"size"`
		Format string `json:"format"` // png (default) or jpeg
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
	size := clamp(in.Size, 256, 16, 1024)
	var img image.Image
	if imageExts[strings.ToLower(filepath.Ext(p))] {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		img, _, err = image.Decode(f)
		f.Close()
		if err != nil {
			return nil, errf(CodeFailed, "cannot read the picture: %v", err)
		}
	} else {
		b, err := thumbnail(ctx, env, p, size)
		if err != nil {
			return nil, err
		}
		if img, _, err = image.Decode(bytes.NewReader(b)); err != nil {
			return nil, errf(CodeFailed, "the thumbnail cannot be read: %v", err)
		}
	}
	img = shrink(img, size)
	var buf bytes.Buffer
	kind := "png"
	if in.Format == "jpeg" || in.Format == "jpg" {
		kind = "jpeg"
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&buf, img)
	}
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	return map[string]any{"type": "image/" + kind, "width": b.Dx(), "height": b.Dy(), "base64": base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

// shrink scales a picture down so its longer side is at most max (an
// average of the pixels each new one covers).
func shrink(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return src
	}
	nw, nh := max, h*max/w
	if h > w {
		nw, nh = w*max/h, max
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := b.Min.Y+y*h/nh, b.Min.Y+(y+1)*h/nh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < nw; x++ {
			x0, x1 := b.Min.X+x*w/nw, b.Min.X+(x+1)*w/nw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			// (at most 4×4 samples a pixel: fast, and smooth enough)
			sy, sx := max1((y1-y0)/4), max1((x1-x0)/4)
			for yy := y0; yy < y1; yy += sy {
				for xx := x0; xx < x1; xx += sx {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), uint8(a/n>>8)
		}
	}
	return dst
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// --- fs.pick, fs.saveAs ---

// Dialog is what a pick or save dialog shows.
type Dialog struct {
	Title    string   `json:"title"`
	Multiple bool     `json:"multiple"`
	Folders  bool     `json:"folders"`
	Types    []string `json:"types"` // endings: ["png", "jpg"]
	Name     string   `json:"name"`  // fs.saveAs: the name to start with
	Folder   string   `json:"folder"`
	save     bool
}

func fsPick(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var d Dialog
	if err := decode(args, &d); err != nil {
		return nil, err
	}
	if err := d.folder(env); err != nil {
		return nil, err
	}
	paths, err := dialog(ctx, env, d)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errf(CodeCancelled, "nothing was picked")
	}
	return map[string]any{"paths": paths}, nil
}

func fsSaveAs(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var d Dialog
	if err := decode(args, &d); err != nil {
		return nil, err
	}
	if err := d.folder(env); err != nil {
		return nil, err
	}
	d.save, d.Multiple, d.Folders = true, false, false
	paths, err := dialog(ctx, env, d)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errf(CodeCancelled, "no place was chosen")
	}
	return map[string]any{"path": paths[0]}, nil
}

func (d *Dialog) folder(env *Env) error {
	if d.Folder == "" {
		return nil
	}
	p, err := env.abs(d.Folder)
	if err != nil {
		return err
	}
	d.Folder = p
	return nil
}
