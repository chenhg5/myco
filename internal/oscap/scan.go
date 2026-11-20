package oscap

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// fs.scan: what takes the room in a folder (or a whole disk), as a tree
// of folders and files by size, and the largest files. Folders are read
// in parallel. The tree keeps what matters: children smaller than a share
// of the whole (min_fraction) are folded into their parent's "smaller
// items", so a scan of a disk stays a few thousand nodes however many
// files there are.

// Node is a folder or file in a scan: its total and, to the depth asked,
// what is in it, largest first.
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
	// Smaller items folded together (left out of Children): how many, and their totals.
	More      int   `json:"more,omitempty"`
	MoreSize  int64 `json:"more_size,omitempty"`
	MoreDisk  int64 `json:"more_disk,omitempty"`
	MoreFiles int64 `json:"more_files,omitempty"`
	// Unreadable: it (or something in it) could not be read (no permission).
	Unreadable bool `json:"unreadable,omitempty"`
}

// BigFile is one of the largest files found.
type BigFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Disk     int64  `json:"disk"`
	Modified int64  `json:"modified"`
}

type scanner struct {
	ctx      context.Context
	emit     Emit
	depth    int
	top      int
	minSize  int64
	fraction float64
	oneFS    bool
	perLevel int
	sem      chan struct{}

	files, dirs, bytes, disk atomic.Int64

	mu       sync.Mutex
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
		// Depth: how many levels of the tree to give (default 2, at most 12).
		Depth int `json:"depth"`
		// Top: how many of the largest files to give (default 50, at most 2000).
		Top int `json:"top"`
		// MinSize: files smaller than this are not in Top.
		MinSize int64 `json:"min_size"`
		// MinFraction: children taking less than this share of the whole
		// (0.001 = 0.1%) are folded into "smaller items" (default 0: none).
		MinFraction float64 `json:"min_fraction"`
		// PerLevel: at most this many children per folder (default 200).
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
	if in.MinFraction < 0 || in.MinFraction >= 1 {
		return nil, errf(CodeInvalid, "min_fraction is a share between 0 and 1 (0.001 is 0.1%%)")
	}
	workers := runtime.NumCPU() * 2
	if workers < 4 {
		workers = 4
	}
	if workers > 32 {
		workers = 32
	}
	s := &scanner{ctx: ctx, emit: emit, depth: clamp(in.Depth, 2, 0, 12), top: clamp(in.Top, 50, 0, 2000), minSize: in.MinSize,
		fraction: in.MinFraction, oneFS: in.OneFilesystem == nil || *in.OneFilesystem, perLevel: clamp(in.PerLevel, 200, 1, 5000),
		sem: make(chan struct{}, workers), seen: map[[2]uint64]bool{}}
	start := time.Now()
	roots := []*Node{}
	stop := make(chan struct{})
	go func() { // progress, four times a second
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.progress()
			case <-stop:
				return
			}
		}
	}()
	defer close(stop)
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
		if s.fraction > 0 {
			s.prune(n, int64(s.fraction*float64(n.Disk)))
		}
		roots = append(roots, n)
	}
	s.progress()
	if s.skipped == nil {
		s.skipped = []string{}
	}
	if s.largest == nil {
		s.largest = []BigFile{}
	}
	return map[string]any{
		"roots": roots, "largest": s.largest, "files": s.files.Load(), "dirs": s.dirs.Load(), "bytes": s.bytes.Load(), "disk": s.disk.Load(),
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

func (s *scanner) progress() {
	s.mu.Lock()
	cur := s.current
	s.mu.Unlock()
	s.emit("progress", map[string]any{"files": s.files.Load(), "dirs": s.dirs.Load(), "bytes": s.bytes.Load(), "disk": s.disk.Load(), "current": cur})
}

func (s *scanner) skip(p string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nskipped++
	if len(s.skipped) < 100 {
		s.skipped = append(s.skipped, p)
	}
}

// file counts a file; false: a hard link to one counted already.
func (s *scanner) file(p string, fi fs.FileInfo, disk int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := inodeOf(fi); ok && linksOf(fi) > 1 {
		if s.seen[id] {
			return false
		}
		s.seen[id] = true
	}
	if s.top == 0 || fi.Size() < s.minSize {
		return true
	}
	if len(s.largest) == s.top && fi.Size() <= s.largest[len(s.largest)-1].Size {
		return true
	}
	f := BigFile{Path: p, Size: fi.Size(), Disk: disk, Modified: fi.ModTime().UnixMilli()}
	i := sort.Search(len(s.largest), func(i int) bool { return s.largest[i].Size < f.Size })
	s.largest = append(s.largest, BigFile{})
	copy(s.largest[i+1:], s.largest[i:])
	s.largest[i] = f
	if len(s.largest) > s.top {
		s.largest = s.largest[:s.top]
	}
	return true
}

func (s *scanner) walk(p string, fi fs.FileInfo, dev uint64, level int) *Node {
	n := &Node{Path: p, Name: fi.Name(), Dir: fi.IsDir(), Modified: fi.ModTime().UnixMilli()}
	if !fi.IsDir() {
		n.Size, n.Disk = fi.Size(), diskOf(fi)
		if fi.Mode().IsRegular() {
			n.Files = 1
			if !s.file(p, fi, n.Disk) {
				n.Disk = 0 // (a hard link to a file already counted)
			}
			s.files.Add(1)
			s.bytes.Add(n.Size)
		}
		s.disk.Add(n.Disk)
		return n
	}
	s.dirs.Add(1)
	s.mu.Lock()
	s.current = p
	s.mu.Unlock()
	des, err := os.ReadDir(p)
	if err != nil {
		s.skip(p)
		n.Unreadable = true
	}
	kids := make([]*Node, len(des))
	var wg sync.WaitGroup
	for i, de := range des {
		if s.ctx.Err() != nil {
			break
		}
		cp := filepath.Join(p, de.Name())
		cfi, err := de.Info()
		if err != nil {
			s.skip(cp)
			continue
		}
		if cfi.IsDir() {
			if s.oneFS {
				if d, ok := devOf(cfi); ok && d != dev {
					continue
				}
			}
			// another worker when one is free; otherwise here
			select {
			case s.sem <- struct{}{}:
				wg.Add(1)
				go func(i int, cp string, cfi fs.FileInfo) {
					defer func() { <-s.sem; wg.Done() }()
					kids[i] = s.walk(cp, cfi, dev, level+1)
				}(i, cp, cfi)
				continue
			default:
			}
		}
		kids[i] = s.walk(cp, cfi, dev, level+1)
	}
	wg.Wait()
	keep := kids[:0]
	for _, k := range kids {
		if k == nil {
			continue
		}
		n.Size += k.Size
		n.Disk += k.Disk
		n.Files += k.Files
		if k.Unreadable {
			n.Unreadable = true
		}
		if level < s.depth {
			if level+1 >= s.depth {
				k.Children, k.More = nil, 0
			}
			keep = append(keep, k)
		}
	}
	if len(keep) > 0 {
		sort.Slice(keep, func(a, b int) bool { return keep[a].Disk > keep[b].Disk })
		// (folded as it goes: with the total so far the share is never more
		// than the final one, so nothing the end would keep is lost)
		floor := int64(0)
		if s.fraction > 0 {
			floor = int64(s.fraction * float64(s.disk.Load()))
		}
		n.Children = keep
		s.fold(n, floor)
	}
	return n
}

// fold moves the children below floor, and those past PerLevel, into the
// node's "smaller items".
func (s *scanner) fold(n *Node, floor int64) {
	cut := len(n.Children)
	for cut > 0 && n.Children[cut-1].Disk < floor {
		cut--
	}
	if cut > s.perLevel {
		cut = s.perLevel
	}
	for _, k := range n.Children[cut:] {
		n.More++
		n.MoreSize += k.Size
		n.MoreDisk += k.Disk
		n.MoreFiles += k.Files
	}
	if cut == 0 {
		n.Children = nil
	} else {
		n.Children = n.Children[:cut:cut]
	}
}

// prune folds the whole tree with the final total.
func (s *scanner) prune(n *Node, floor int64) {
	if len(n.Children) == 0 {
		return
	}
	s.fold(n, floor)
	for _, k := range n.Children {
		s.prune(k, floor)
	}
}
