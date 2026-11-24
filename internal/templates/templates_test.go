package templates_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/templates"
)

// Every official template reads, has its page, the agent's
// guide and sample data that its own data structure accepts.
func TestOfficialTemplatesAreWhole(t *testing.T) {
	list, err := templates.FromDir(os.DirFS("official"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, o := range list {
		m := o.Manifest
		ids = append(ids, m.ID)
		if m.Category == "" {
			t.Errorf("%s has no category (its shelf in the App)", m.ID)
		}
		if m.Icon == "" || !strings.Contains(string(o.Files[m.Icon]), "<svg") {
			t.Errorf("%s has no icon of its own", m.ID)
		}
		for _, f := range []string{m.Entry, m.Agent, m.Sample} {
			if len(o.Files[f]) == 0 {
				t.Errorf("%s has no %s", m.ID, f)
			}
		}
		// The App shows what it looks like: a screenshot, light and dark.
		if len(o.Files["shots/1.webp"]) == 0 || len(o.Files["shots/1.dark.webp"]) == 0 {
			t.Errorf("%s has no screenshots (node templates/shots.mjs %s)", m.ID, strings.TrimPrefix(m.ID, "myco/"))
		}
		// Its page loads what is in it, nothing from the internet.
		page := string(o.Files[m.Entry])
		if !strings.Contains(page, "<script") || strings.Contains(page, "http://") || strings.Contains(page, "https://") {
			t.Errorf("%s: its page should load its own scripts only", m.ID)
		}
		var sample map[string][]map[string]json.RawMessage
		if err := json.Unmarshal(o.Files[m.Sample], &sample); err != nil {
			t.Fatalf("%s sample: %v", m.ID, err)
		}
		for coll, recs := range sample {
			col, ok := m.Data[coll]
			if !ok {
				t.Errorf("%s sample: no collection %s", m.ID, coll)
				continue
			}
			for _, r := range recs {
				delete(r, "id")
				raw, _ := json.Marshal(r)
				if err := col.CheckRecord(coll, string(raw)); err != nil {
					t.Errorf("%s sample %s: %v", m.ID, coll, err)
				}
			}
		}
	}
	var want []string
	for _, id := range templates.Order {
		for _, x := range ids {
			if x == id {
				want = append(want, id)
			}
		}
	}
	if len(ids) < 4 || strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("official templates: %v", ids)
	}
}

// The binary carries the Builtin apps, as official/ has them now: make
// templates (go run ./internal/templates/genbuiltin) after changing one.
func TestBuiltinIsCurrent(t *testing.T) {
	want, err := templates.PackBuiltin(os.DirFS("official"))
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile("builtin.zip")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(have, want) {
		t.Fatal("internal/templates/builtin.zip is stale: run go run ./internal/templates/genbuiltin (make templates does) and commit it")
	}
	list, err := templates.Official()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, o := range list {
		got[o.Manifest.ID] = true
	}
	for _, a := range templates.Builtin {
		if !got["myco/"+a] {
			t.Errorf("myco/%s is not in the binary", a)
		}
	}
	if len(list) != len(templates.Builtin) {
		t.Errorf("the binary carries %d apps, want the %d builtin ones", len(list), len(templates.Builtin))
	}
}

// Screenshots stay small: at most 1280 wide (templates/shots.mjs).
func TestShotsAreSmall(t *testing.T) {
	list, err := templates.FromDir(os.DirFS("official"))
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range list {
		for name, b := range o.Files {
			if !strings.HasPrefix(name, "shots/") || !strings.HasSuffix(name, ".webp") {
				continue
			}
			if w := webpWidth(b); w > 1280 {
				t.Errorf("%s %s is %d wide: at most 1280 (node templates/shots.mjs %s)", o.Manifest.ID, name, w, strings.TrimPrefix(o.Manifest.ID, "myco/"))
			}
		}
	}
}

// webpWidth reads a WebP picture's width from its header (0: unknown).
func webpWidth(b []byte) int {
	if len(b) < 30 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0
	}
	switch string(b[12:16]) {
	case "VP8X":
		return 1 + (int(b[24]) | int(b[25])<<8 | int(b[26])<<16)
	case "VP8 ":
		return (int(b[26]) | int(b[27])<<8) & 0x3fff
	case "VP8L":
		return 1 + (int(b[21])|int(b[22])<<8)&0x3fff
	}
	return 0
}
