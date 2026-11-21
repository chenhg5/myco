package oscap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestScanFoldsSmallItems(t *testing.T) {
	env := testEnv(t)
	// one big file and fifty small ones: with min_fraction the small ones are folded
	call(t, env, "fs.write", map[string]any{"path": "~/s/big.bin", "content": strings.Repeat("x", 400000)})
	for i := 0; i < 50; i++ {
		call(t, env, "fs.write", map[string]any{"path": fmt.Sprintf("~/s/small/f%02d", i), "content": strings.Repeat("x", 1000)})
	}
	js := &Jobs{Env: env}
	j, _ := js.Start("fs.scan", json.RawMessage(`{"path":"~/s","depth":3,"min_fraction":0.05}`))
	b := wait(t, js, j.ID)
	out, _ := json.Marshal(b.Value)
	var v struct{ Roots []Node }
	_ = json.Unmarshal(out, &v)
	root := v.Roots[0]
	// (the folder of small files is over 5% of the whole, so it stays:
	// 1 KB each, or a disk block each where blocks are counted; each file
	// in it is under 5% and is folded. Windows counts no blocks, so files
	// of a byte would fold the folder too.)
	if root.Files != 51 || len(root.Children) != 2 || root.Children[0].Name != "big.bin" {
		t.Fatalf("children: %s", out)
	}
	small := root.Children[1]
	if small.Name != "small" || len(small.Children) != 0 || small.More != 50 || small.MoreFiles != 50 || small.MoreDisk != small.Disk {
		t.Fatalf("folded: %s", out)
	}
}

func TestSearch(t *testing.T) {
	env := testEnv(t)
	for _, f := range []string{"~/d/Report 2024.pdf", "~/d/report-old.PDF", "~/d/notes.txt", "~/d/sub/report.docx", "~/d/.hidden/report.pdf"} {
		call(t, env, "fs.write", map[string]any{"path": f, "content": "x"})
	}
	js := &Jobs{Env: env}
	j, _ := js.Start("fs.search", json.RawMessage(`{"path":"~/d","name":"report","ext":["pdf"]}`))
	b := wait(t, js, j.ID)
	out, _ := json.Marshal(b.Value)
	if strings.Count(string(out), `"name":`) != 2 || strings.Contains(string(out), ".hidden") {
		t.Fatalf("name + ext, no hidden: %s", out)
	}
	j, _ = js.Start("fs.search", json.RawMessage(`{"path":"~/d","name":"report*.docx"}`))
	if out, _ = json.Marshal(wait(t, js, j.ID).Value); strings.Count(string(out), `"name":`) != 1 {
		t.Fatalf("pattern: %s", out)
	}
	j, _ = js.Start("fs.search", json.RawMessage(`{"path":"~/d","limit":2,"hidden":true}`))
	b = wait(t, js, j.ID)
	if out, _ = json.Marshal(b.Value); !strings.Contains(string(out), `"truncated":true`) {
		t.Fatalf("limit: %s", out)
	}
	matched := false
	for _, e := range b.Events {
		matched = matched || e.Type == "match"
	}
	if !matched {
		t.Fatal("no match events")
	}
}

func TestWatch(t *testing.T) {
	env := testEnv(t)
	call(t, env, "fs.write", map[string]any{"path": "~/w/a.txt", "content": "1"})
	js := &Jobs{Env: env}
	j, err := js.Start("fs.watch", json.RawMessage(`{"path":"~/w","interval_ms":250}`))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	call(t, env, "fs.write", map[string]any{"path": "~/w/b.txt", "content": "new"})
	call(t, env, "fs.write", map[string]any{"path": "~/w/a.txt", "content": "changed"})
	deadline := time.Now().Add(5 * time.Second)
	var seen []string
	next := 0
	for time.Now().Before(deadline) && len(seen) < 2 {
		b, _ := js.Read(context.Background(), j.ID, next, 500*time.Millisecond)
		next = b.Next
		for _, e := range b.Events {
			if e.Type == "change" {
				for _, c := range e.Data.([]map[string]any) {
					seen = append(seen, fmt.Sprintf("%s %s", c["type"], filepath.Base(c["path"].(string))))
				}
			}
		}
	}
	_ = js.Cancel(j.ID)
	got := strings.Join(seen, ",")
	if !strings.Contains(got, "created b.txt") || !strings.Contains(got, "modified a.txt") {
		t.Fatalf("changes: %v", seen)
	}
}

func TestThumbnail(t *testing.T) {
	env := testEnv(t)
	img := image.NewRGBA(image.Rect(0, 0, 800, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 800; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	_ = os.WriteFile(filepath.Join(env.Home, "pic.png"), buf.Bytes(), 0o644)
	r := call(t, env, "fs.thumbnail", map[string]any{"path": "~/pic.png", "size": 200})
	if r["width"].(float64) != 200 || r["height"].(float64) != 100 {
		t.Fatalf("size: %v %v", r["width"], r["height"])
	}
	b, _ := base64.StdEncoding.DecodeString(r["base64"].(string))
	if _, err := png.Decode(bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}
}

func TestUsageAndApps(t *testing.T) {
	env := testEnv(t)
	u := call(t, env, "system.usage", nil)
	if runtime.GOOS == "linux" && (u["memory_total"].(float64) <= 0 || u["cpu"].(float64) < 0 || u["cpu"].(float64) > 100) {
		t.Fatalf("usage: %v", u)
	}
	if runtime.GOOS == "linux" {
		dir := filepath.Join(env.Home, ".local", "share", "applications")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "zz-myco-test.desktop"), []byte("[Desktop Entry]\nType=Application\nName=Myco Test\nExec=true\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "zz-hidden.desktop"), []byte("[Desktop Entry]\nType=Application\nName=Hidden\nNoDisplay=true\n"), 0o644)
		v, err := callRaw(env, "system.apps", nil)
		if err != nil {
			t.Fatal(err)
		}
		found, hidden := false, false
		for _, a := range v.([]AppInfo) {
			found = found || a.Name == "Myco Test" && a.ID == "zz-myco-test"
			hidden = hidden || a.Name == "Hidden"
		}
		if !found || hidden {
			t.Fatalf("apps: %+v", v)
		}
	}
	if _, err := callRaw(env, "system.settings", map[string]string{"pane": "nope"}); err == nil || err.Code != CodeInvalid {
		t.Fatalf("unknown pane: %v", err)
	}
}

func TestPlist(t *testing.T) {
	kv := xmlPlistStrings([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Visual Studio Code</string>
  <key>CFBundleDocumentTypes</key><array><dict><key>CFBundleTypeName</key><string>nested</string></dict></array>
  <key>LSRequiresNativeExecution</key><true/>
  <key>CFBundleShortVersionString</key><string>1.96.2</string>
</dict></plist>`))
	if kv["CFBundleName"] != "Visual Studio Code" || kv["CFBundleShortVersionString"] != "1.96.2" || kv["CFBundleTypeName"] != "" {
		t.Fatalf("plist: %v", kv)
	}
}
