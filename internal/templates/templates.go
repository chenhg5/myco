// Package templates holds myco's official templates (templates.md): pages
// the hub offers in every workspace's library, made with the same parts as
// anyone's (myco.json, the page, agent.md, sample.json). Their sources are
// in templates/ (React, bundled with Vite: make templates), built into
// official/.
//
// The binary carries only the Builtin ones, compressed (builtin.zip, made
// from official/ by make templates): a hub fetches all of them from the
// "apps" release (Catalog) and keeps what it fetched, so the rest only
// wait for its first fetch. Carrying all of them made every download of
// myco (and the Mac app, twice) 40 MB bigger.
package templates

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// Builtin are the official apps the binary carries: the everyday ones, so
// a new hub that cannot reach GitHub still has a useful library.
var Builtin = []string{"table", "base", "doc", "slides", "meeting", "scheduling", "issues", "todo", "notes", "whiteboard", "mindmap"}

//go:embed builtin.zip
var builtinZip []byte

// Order is how the library lists the apps it knows within a shelf (as a
// person would reach for them); shelves come first (domain.TemplateCategories).
var Order = []string{
	"myco/table", "myco/base", "myco/doc", "myco/slides", "myco/meeting", "myco/tutor", "myco/scheduling",
	"myco/database", "myco/code", "myco/issues", "myco/devtools",
	"myco/todo", "myco/notes", "myco/disk", "myco/stocks", "myco/proxy-console", "myco/server-console", "myco/remote-desktop",
	"myco/ledger", "myco/habit", "myco/reader", "myco/price-compare", "myco/cinema", "myco/podcast", "myco/personality",
	"myco/palette", "myco/aigc-canvas", "myco/design-studio", "myco/logo-studio", "myco/whiteboard", "myco/mindmap", "myco/recorder", "myco/video", "myco/motion", "myco/photo",
	"myco/arcade", "myco/ocean", "myco/gomoku", "myco/sudoku",
}

func rank(id string) int {
	for i, x := range Order {
		if x == id {
			return i
		}
	}
	return len(Order)
}

// Official returns the official templates the binary carries (Builtin).
func Official() ([]app.OfficialTemplate, error) {
	out, err := (&Catalog{}).read(builtinZip)
	if err != nil {
		return nil, err
	}
	sortOfficial(out)
	return out, nil
}

// FromDir reads official templates from a folder holding one folder an app
// (official/ in the source tree): every one, for checks and tools.
func FromDir(fsys fs.FS) ([]app.OfficialTemplate, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []app.OfficialTemplate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := e.Name()
		t := app.OfficialTemplate{Files: map[string][]byte{}}
		err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			t.Files[p[len(dir)+1:]] = b
			return nil
		})
		if err != nil {
			return nil, err
		}
		m, err := domain.ParseManifest(t.Files["myco.json"])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		t.Manifest = m
		out = append(out, t)
	}
	sortOfficial(out)
	return out, nil
}

// PackBuiltin makes builtin.zip from official/ (fsys): the Builtin apps,
// one folder each as in the "apps" release, compressed as far as deflate
// goes, and the same bytes for the same files (sorted, no times), so the
// committed archive changes only when an app does.
func PackBuiltin(fsys fs.FS) ([]byte, error) {
	var names []string
	for _, a := range Builtin {
		if _, err := fs.Stat(fsys, path.Join(a, "myco.json")); err != nil {
			return nil, fmt.Errorf("builtin app %s: %w", a, err)
		}
		err := fs.WalkDir(fsys, a, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || path.Base(p)[0] == '.' {
				return err
			}
			names = append(names, p)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(names)
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	w.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestCompression)
	})
	for _, n := range names {
		b, err := fs.ReadFile(fsys, n)
		if err != nil {
			return nil, err
		}
		f, err := w.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(b); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sortOfficial puts them in the library's order: by shelf, then as Order
// has them (apps it does not know after those it does), then by name.
func sortOfficial(list []app.OfficialTemplate) {
	shelf := func(c string) int {
		for i, x := range domain.TemplateCategories {
			if x == c {
				return i
			}
		}
		return len(domain.TemplateCategories)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i].Manifest, list[j].Manifest
		if sa, sb := shelf(a.Category), shelf(b.Category); sa != sb {
			return sa < sb
		}
		if ra, rb := rank(a.ID), rank(b.ID); ra != rb {
			return ra < rb
		}
		return a.ID < b.ID
	})
}
