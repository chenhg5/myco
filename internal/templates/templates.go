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
	order := map[string]int{"myco/table": 0, "myco/base": 1, "myco/doc": 2, "myco/slides": 3}
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].Manifest.ID] < order[out[j].Manifest.ID] })
	return out, nil
}
