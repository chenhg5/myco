package oscap

import (
	"context"
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
	// who speaks when: three voices take turns in the test recording
	j, _ = js.Start("speech.diarize", json.RawMessage(`{"path":`+mustJSON(in)+`}`))
	b = waitFor(t, js, j.ID, 2*time.Minute)
	if b.Error != nil {
		t.Fatal(b.Error)
	}
	t.Logf("diarize: %s", mustJSON(b.Value))
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

func TestTurns(t *testing.T) {
	if tt, ok := parseTurn("8.148 -- 13.548 speaker_03"); !ok || tt.Start != 8.148 || tt.End != 13.548 || tt.Speaker != 3 {
		t.Fatalf("%+v %v", tt, ok)
	}
	if _, ok := parseTurn("progress 100.00%"); ok {
		t.Fatal("progress is not a turn")
	}
	got := renumber([]Turn{{Start: 9, Speaker: 3}, {Start: 1, Speaker: 2}, {Start: 5, Speaker: 0}, {Start: 12, Speaker: 2}})
	want := []int{0, 1, 2, 0}
	for i, tt := range got {
		if tt.Speaker != want[i] {
			t.Fatalf("%+v", got)
		}
	}
}

// fakeCloud stands in for the App's cloud speech.
type fakeCloud struct {
	code, why string
	parts     int
	err       error
}

func (f *fakeCloud) Status(engine string) (string, string) { return f.code, f.why }
func (f *fakeCloud) Transcribe(ctx context.Context, engine string, wav []byte, lang string) ([]Segment, error) {
	f.parts++
	if f.err != nil {
		return nil, f.err
	}
	if len(wav) < 44 || string(wav[:4]) != "RIFF" {
		return nil, errf(CodeInvalid, "not a WAV")
	}
	return []Segment{{Start: 0.5, End: 1.25, Text: "让 Claude 看一下"}}, nil
}

func TestSpeechTranscribeCloud(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "a.wav")
	if err := writePCM16(wav, make([]int16, 3*RecordRate)); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"path": wav, "engine": "doubao", "offset": 10})
	var segs []any
	emit := func(kind string, data any) {
		if kind == "segment" {
			segs = append(segs, data)
		}
	}
	// No App: not here.
	if _, err := speechTranscribe(context.Background(), &Env{Home: dir}, args, emit); AsError(err).Code != CodeUnsupported {
		t.Fatalf("without the App: %v", err)
	}
	// No key: says so, with the code.
	env := &Env{Home: dir, CloudSpeech: &fakeCloud{code: CodeNoCredentials, why: "add the key"}}
	if _, err := speechTranscribe(context.Background(), env, args, emit); AsError(err).Code != CodeNoCredentials {
		t.Fatalf("without a key: %v", err)
	}
	// The service not on: its code comes through.
	env.CloudSpeech = &fakeCloud{err: errf(CodeServiceNotEnabled, "turn it on")}
	if _, err := speechTranscribe(context.Background(), env, args, emit); AsError(err).Code != CodeServiceNotEnabled {
		t.Fatalf("not turned on: %v", err)
	}
	// Recognized: segments with the offset, as this computer's are.
	fc := &fakeCloud{}
	env.CloudSpeech = fc
	out, err := speechTranscribe(context.Background(), env, args, emit)
	if err != nil {
		t.Fatal(err)
	}
	res := out.(map[string]any)
	got := res["segments"].([]Segment)
	if len(got) != 1 || got[0].Start != 10.5 || got[0].End != 11.25 || res["engine"] != "doubao" || fc.parts != 1 || len(segs) != 1 {
		t.Fatalf("result: %+v parts=%d", res, fc.parts)
	}
	// An unknown engine.
	bad, _ := json.Marshal(map[string]any{"path": wav, "engine": "whisper"})
	if _, err := speechTranscribe(context.Background(), env, bad, emit); AsError(err).Code != CodeInvalid {
		t.Fatalf("unknown engine: %v", err)
	}
	// speech.status lists it.
	st, _ := speechStatus(context.Background(), env, nil, emit)
	if c := st.(map[string]any)["cloud"].(map[string]any)["doubao"].(map[string]any); c["ready"] != true {
		t.Fatalf("status: %v", c)
	}
}
