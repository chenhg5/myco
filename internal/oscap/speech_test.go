package oscap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSegments(t *testing.T) {
	out := "Started\nElapsed seconds: 0.7 s\n0.742 -- 5.612: 开饭时间早上九点至下午五点\n8.166 -- 14.220: THE CHIEFTAIN\n15 -- 16: \n"
	got := parseSegments(out)
	if len(got) != 2 || got[0].Start != 0.742 || got[0].End != 5.612 || got[0].Text != "开饭时间早上九点至下午五点" || got[1].Text != "THE CHIEFTAIN" {
		t.Fatalf("%+v", got)
	}
}

func TestSplitQuiet(t *testing.T) {
	// 10 s of sound with a quiet moment at 5.5 s; parts of about 4 s
	s := make([]int16, 10*RecordRate)
	for i := range s {
		s[i] = 8000
	}
	for i := 5*RecordRate + RecordRate/2; i < 6*RecordRate; i++ {
		s[i] = 0
	}
	parts := splitQuiet(s, 6*RecordRate)
	if len(parts) != 2 || parts[0][0] != 0 || parts[1][1] != len(s) || parts[0][1] != parts[1][0] {
		t.Fatalf("%v", parts)
	}
	if c := parts[0][1]; c < 5*RecordRate+RecordRate/2 || c > 6*RecordRate {
		t.Fatalf("cut at %.2f s, not in the quiet", float64(c)/RecordRate)
	}
	if p := splitQuiet(s[:RecordRate], 6*RecordRate); len(p) != 1 {
		t.Fatal(p)
	}
}

func TestSpeechStatusNotInstalled(t *testing.T) {
	if engineArchive() == "" {
		t.Skip("no engine for this system")
	}
	env := testEnv(t)
	v := call(t, env, "speech.status", nil)
	if v["ready"] != false || v["engine"] != false {
		t.Fatalf("%v", v)
	}
	if _, err := callRaw(env, "speech.transcribe", map[string]any{"path": "~/x.wav"}); err == nil || !strings.Contains(err.Message, "job") {
		t.Fatalf("a job, not a call: %v", err)
	}
	js := &Jobs{Env: env}
	j, _ := js.Start("speech.transcribe", json.RawMessage(`{"path":"~/x.wav"}`))
	if b := wait(t, js, j.ID); b.Error == nil || b.Error.Code != CodeNotFound {
		t.Fatalf("want not_found (install first): %+v", b.Error)
	}
}

// TestSpeechEndToEnd installs from a folder laid out as GitHub's release
// downloads (MYCO_SPEECH_TEST_MIRROR: <tag>/<file>, or "github" for the
// real ones) and transcribes MYCO_SPEECH_TEST_WAV (any format the system
// reads). It needs the real files (200 MB), so it runs only when they are
// given.
func TestSpeechEndToEnd(t *testing.T) {
	dir, wav := os.Getenv("MYCO_SPEECH_TEST_MIRROR"), os.Getenv("MYCO_SPEECH_TEST_WAV")
	if dir == "" || wav == "" {
		t.Skip("MYCO_SPEECH_TEST_MIRROR and MYCO_SPEECH_TEST_WAV not set")
	}
	if dir != "github" { // ("github": the real downloads)
		srv := httptest.NewServer(http.StripPrefix("/k2-fsa/sherpa-onnx/releases/download", http.FileServer(http.Dir(dir))))
		defer srv.Close()
		t.Setenv("MYCO_SPEECH_MIRROR", srv.URL)
	}
	env := testEnv(t)
	js := &Jobs{Env: env}
	j, err := js.Start("speech.install", nil)
	if err != nil {
		t.Fatal(err)
	}
	b := waitFor(t, js, j.ID, 3*time.Minute)
	if b.Error != nil {
		t.Fatal(b.Error)
	}
	if st := call(t, env, "speech.status", nil); st["ready"] != true {
		t.Fatalf("not ready after install: %v", st)
	}
	in := filepath.Join(env.Home, "talk"+filepath.Ext(wav))
	data, _ := os.ReadFile(wav)
	_ = os.WriteFile(in, data, 0o600)
	j, _ = js.Start("speech.transcribe", json.RawMessage(`{"path":`+mustJSON(in)+`,"offset":100}`))
	b = waitFor(t, js, j.ID, 2*time.Minute)
	if b.Error != nil {
		t.Fatal(b.Error)
	}
	var v struct {
		Segments []Segment `json:"segments"`
	}
	_ = json.Unmarshal([]byte(mustJSON(b.Value)), &v)
	if len(v.Segments) == 0 || v.Segments[0].Start < 100 {
		t.Fatalf("%s", mustJSON(b.Value))
	}
	for _, s := range v.Segments {
		t.Logf("%.2f–%.2f %s", s.Start, s.End, s.Text)
	}
	segs := 0
	for _, e := range b.Events {
		if e.Type == "segment" {
			segs++
		}
	}
	if segs != len(v.Segments) {
		t.Fatalf("%d segment events, %d segments", segs, len(v.Segments))
	}
}

func waitFor(t *testing.T, js *Jobs, id string, d time.Duration) Batch {
	t.Helper()
	end := time.Now().Add(d)
	var all []Event
	next := 0
	for time.Now().Before(end) {
		b, err := js.Read(t.Context(), id, next, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, b.Events...)
		next = b.Next
		if b.Done {
			b.Events = all
			return b
		}
	}
	t.Fatalf("job %s did not end in %s", id, d)
	return Batch{}
}

func TestSystemProxy(t *testing.T) {
	mac := "<dictionary> {\n  HTTPEnable : 1\n  HTTPPort : 10808\n  HTTPProxy : 127.0.0.1\n  HTTPSEnable : 1\n  HTTPSPort : 10809\n  HTTPSProxy : 127.0.0.1\n}\n"
	if u := scutilProxy(mac); u == nil || u.String() != "http://127.0.0.1:10809" {
		t.Fatalf("mac: %v", u)
	}
	if u := scutilProxy("<dictionary> {\n  HTTPSEnable : 0\n}\n"); u != nil {
		t.Fatalf("off: %v", u)
	}
	win := "\n    ProxyEnable    REG_DWORD    0x1\n    ProxyServer    REG_SZ    http=10.0.0.1:8080;https=10.0.0.1:8443\n"
	if u := winProxy(win); u == nil || u.String() != "http://10.0.0.1:8443" {
		t.Fatalf("win: %v", u)
	}
	if u := winProxy("    ProxyEnable    REG_DWORD    0x0\n    ProxyServer    REG_SZ    1.2.3.4:80\n"); u != nil {
		t.Fatalf("win off: %v", u)
	}
	if u := winProxy("    ProxyEnable    REG_DWORD    0x1\n    ProxyServer    REG_SZ    1.2.3.4:80\n"); u == nil || u.Host != "1.2.3.4:80" {
		t.Fatalf("win plain: %v", u)
	}
}
