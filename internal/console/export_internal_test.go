package console

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An export the app rendered is saved to Downloads under its name (never
// another kind of file), and shown.
func TestExportSavesToDownloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	var shown string
	was := revealExport
	revealExport = func(p string) { shown = p }
	defer func() { revealExport = was }()
	b := &Backend{Store: NewStore(filepath.Join(t.TempDir(), "hubs.json"))}
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	post := func(body string) (*http.Response, []byte) {
		resp, err := http.Post(srv.URL+"/app/export", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp, raw
	}
	resp, raw := post(`{"name":"和 pm-codex 的对话.md","content":"# 对话导出\n"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}
	want := filepath.Join(home, "Downloads", "和 pm-codex 的对话.md")
	if got, _ := os.ReadFile(want); string(got) != "# 对话导出\n" || shown != want {
		t.Fatalf("saved %q, shown %q", got, shown)
	}
	// Again: a second file, not overwritten.
	if resp, raw := post(`{"name":"和 pm-codex 的对话.md","content":"x"}`); resp.StatusCode != 200 || !strings.Contains(string(raw), "(1).md") {
		t.Fatalf("second: %d %s", resp.StatusCode, raw)
	}
	if resp, _ := post(`{"name":"run.sh","content":"x"}`); resp.StatusCode != 400 {
		t.Fatalf("another kind of file: %d", resp.StatusCode)
	}
}

// A page saves its video (视频剪辑's export) to Downloads, past the 20 MB a
// document may have; programs are still refused.
func TestPageDownloadSavesVideo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	was := revealExport
	revealExport = func(string) {}
	defer func() { revealExport = was }()
	b := &Backend{Store: NewStore(filepath.Join(t.TempDir(), "hubs.json"))}
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	post := func(name string, body io.Reader) (int, string) {
		resp, err := http.Post(srv.URL+"/app/page-download?name="+name, "application/octet-stream", body)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	big := strings.Repeat("v", maxPageDownload+1024)
	if code, raw := post("cut.mp4", strings.NewReader(big)); code != 200 || !strings.Contains(raw, "cut.mp4") {
		t.Fatalf("video: %d %s", code, raw)
	}
	if st, err := os.Stat(filepath.Join(home, "Downloads", "cut.mp4")); err != nil || st.Size() != int64(len(big)) {
		t.Fatalf("saved video: %v %v", st, err)
	}
	if code, _ := post("report.pdf", strings.NewReader(big)); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a document past 20 MB: %d", code)
	}
	if _, err := os.Stat(filepath.Join(home, "Downloads", "report.pdf")); !os.IsNotExist(err) {
		t.Fatalf("a refused file is not left behind: %v", err)
	}
	if code, _ := post("tool.exe", strings.NewReader("MZ")); code != 400 {
		t.Fatalf("a program: %d", code)
	}
}
