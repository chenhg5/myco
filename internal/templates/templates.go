// Package templates holds myco's official templates (templates.md): pages
// the hub offers in every workspace's library, made with the same parts as
// anyone's (myco.json, the page, agent.md, sample.json) plus kit.js and
// kit.css, which they share.
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

// shared are put in every official template.
var shared = []string{"kit.js", "kit.css"}

// Official returns the official templates, read from the binary.
func Official() ([]app.OfficialTemplate, error) {
	entries, err := fs.ReadDir(files, "official")
	if err != nil {
		return nil, err
	}
	common := map[string][]byte{}
	for _, f := range shared {
		b, err := files.ReadFile(path.Join("official", f))
		if err != nil {
			return nil, err
		}
		common[f] = b
	}
	var out []app.OfficialTemplate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join("official", e.Name())
		t := app.OfficialTemplate{Files: map[string][]byte{}}
		for k, v := range common {
			t.Files[k] = v
		}
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
	order := map[string]int{"myco/table": 0, "myco/base": 1, "myco/doc": 2}
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].Manifest.ID] < order[out[j].Manifest.ID] })
	return out, nil
}
