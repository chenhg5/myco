// Package templates holds myco's official templates (templates.md): pages
// the hub offers in every workspace's library, made with the same parts as
// anyone's (myco.json, the page, agent.md, sample.json). Their sources are
// in templates/ (React, bundled with Vite: make templates).
package templates

import (
	"embed"
	"io/fs"
	"path"
	"sort"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

//go:embed official
var files embed.FS

// Order is how the library lists them: by shelf (office, tools, life,
// creative, games), then as a person would reach for them.
var Order = []string{
	"myco/table", "myco/base", "myco/doc", "myco/slides",
	"myco/todo", "myco/poll",
	"myco/ledger", "myco/habit",
	"myco/whiteboard", "myco/mindmap",
	"myco/gomoku", "myco/sudoku",
}

func rank(id string) int {
	for i, x := range Order {
		if x == id {
			return i
		}
	}
	return len(Order)
}

// Official returns the official templates, read from the binary.
func Official() ([]app.OfficialTemplate, error) {
	entries, err := fs.ReadDir(files, "official")
	if err != nil {
		return nil, err
	}
	var out []app.OfficialTemplate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join("official", e.Name())
		t := app.OfficialTemplate{Files: map[string][]byte{}}
		err := fs.WalkDir(files, dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := files.ReadFile(p)
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
			return nil, err
		}
		t.Manifest = m
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i].Manifest.ID) < rank(out[j].Manifest.ID) })
	return out, nil
}
