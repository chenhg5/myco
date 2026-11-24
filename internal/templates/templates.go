// Package templates loads official apps from verified release bundles.
// Sources, the page SDK and app tests live in github.com/myco-ai/apps.
// Production builds prepare a locked builtin.zip and generated embed file;
// unprepared developer builds have no bundled apps and may fetch the catalog.
package templates

import (
	"fmt"
	"io/fs"
	"sort"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

// builtinZip is populated only by the ignored generated production embed file.
// Test fixtures are loaded explicitly through hub options, never embedded here.
var builtinZip []byte
var bundledVersion string

// Order is how the library lists the apps it knows within a shelf (as a
// person would reach for them); shelves come first (domain.TemplateCategories).
var Order = []string{
	"myco/table", "myco/base", "myco/doc", "myco/slides", "myco/meeting", "myco/tutor", "myco/scheduling",
	"myco/database", "myco/code", "myco/issues", "myco/devtools",
	"myco/todo", "myco/notes", "myco/disk", "myco/stocks", "myco/proxy-console", "myco/server-console", "myco/remote-desktop",
	"myco/ledger", "myco/habit", "myco/reader", "myco/price-compare", "myco/cinema", "myco/podcast", "myco/personality",
	"myco/palette", "myco/aigc-canvas", "myco/design-studio", "myco/logo-studio", "myco/whiteboard", "myco/mindmap", "myco/recorder", "myco/video", "myco/motion", "myco/photo", "myco/media-kit",
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

// Official returns the official templates the binary carries, or none in an unprepared checkout.
func Official() ([]app.OfficialTemplate, error) {
	if len(builtinZip) == 0 {
		return nil, nil
	}
	out, err := (&Catalog{}).read(builtinZip)
	if err != nil {
		return nil, err
	}
	sortOfficial(out)
	return out, nil
}

// FromDir reads official templates from a folder holding one folder an app
// (for example, dist/official in the apps repository), for checks and tools.
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
