package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myco-ai/myco/internal/harness"
)

// Model is a model Codex offers in its picker.
type Model struct {
	ID      string   // its slug: what --model / -c model= takes
	Name    string   // its display name
	Efforts []string // the reasoning efforts it supports
}

// MaxModels bounds the list.
const MaxModels = 40

// CachedModels lists the models this computer's Codex offers, from the
// list it keeps after asking its provider (models_cache.json in CODEX_HOME,
// newest Codex versions); nil when there is none. The order is Codex's own
// (priority); hidden ones are left out.
func CachedModels() []Model {
	return ModelsIn(harness.HomeDir("CODEX_HOME", ".codex"))
}

// ModelsIn reads the models cache in Codex home dir.
func ModelsIn(dir string) []Model {
	b, err := os.ReadFile(filepath.Join(dir, "models_cache.json"))
	if err != nil || len(b) > 4<<20 {
		return nil
	}
	return parseModels(b)
}

func parseModels(b []byte) []Model {
	var f struct {
		Models []struct {
			Slug           string   `json:"slug"`
			DisplayName    string   `json:"display_name"`
			Visibility     string   `json:"visibility"`
			Priority       *float64 `json:"priority"`
			SupportedInAPI *bool    `json:"supported_in_api"`
			Levels         []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	type ranked struct {
		m    Model
		rank float64
		i    int
	}
	var list []ranked
	seen := map[string]bool{}
	for i, x := range f.Models {
		id := strings.TrimSpace(x.Slug)
		if id == "" || seen[id] || len(id) > 80 {
			continue
		}
		switch strings.ToLower(x.Visibility) {
		case "", "list":
		default: // hide, none
			continue
		}
		if x.SupportedInAPI != nil && !*x.SupportedInAPI {
			continue
		}
		seen[id] = true
		m := Model{ID: id, Name: strings.TrimSpace(x.DisplayName)}
		if m.Name == id || len(m.Name) > 80 {
			m.Name = ""
		}
		for _, l := range x.Levels {
			if e := strings.TrimSpace(l.Effort); e != "" && len(e) < 20 {
				m.Efforts = append(m.Efforts, e)
			}
		}
		r := 10_000.0
		if x.Priority != nil {
			r = *x.Priority
		}
		list = append(list, ranked{m, r, i})
	}
	sort.SliceStable(list, func(a, b int) bool { return list[a].rank < list[b].rank })
	var out []Model
	for _, x := range list {
		if len(out) == MaxModels {
			break
		}
		out = append(out, x.m)
	}
	return out
}
