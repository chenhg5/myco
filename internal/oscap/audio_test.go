package oscap

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAudio plays a tone per source, at its own rate, every 20 ms.
type fakeAudio struct{ rates map[string]int }

func (f fakeAudio) Sources(context.Context) ([]AudioSource, error) {
	return []AudioSource{{ID: "mic", Kind: "mic", Name: "mic", Default: true}, {ID: "system", Kind: "system", Name: "system"}}, nil
}

func (f fakeAudio) Capture(ctx context.Context, id string, cb func([]float32, int)) (func(), error) {
	rate, ok := f.rates[id]
	if !ok {
		return nil, errf(CodeInvalid, "no such source: %s", id)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(20 * time.Millisecond)
		defer t.Stop()
		n := 0
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s := make([]float32, rate/50)
				for i := range s {
					s[i] = 0.5 * float32(math.Sin(2*math.Pi*440*float64(n)/float64(rate)))
					n++
				}
				cb(s, rate)
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(stop) }); wg.Wait() }, nil
}

func TestRecord(t *testing.T) {
	env := testEnv(t)
	SetAudio(fakeAudio{rates: map[string]int{"mic": 48000, "system": 16000}})
	defer SetAudio(osAudio())
	if v := call(t, env, "audio.sources", nil); !strings.Contains(mustJSON(v), `"system"`) {
		t.Fatalf("sources: %v", mustJSON(v))
	}
	js := &Jobs{Env: env}
	j, err := js.Start("audio.record", json.RawMessage(`{"dir":"~/rec","tracks":[{"id":"me","source":"mic"},{"id":"them","source":"system"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := js.Input(j.ID, []byte("pause")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	_ = js.Input(j.ID, []byte("resume"))
	time.Sleep(300 * time.Millisecond)
	_ = js.Input(j.ID, []byte("stop"))
	b := wait(t, js, j.ID)
	if b.Error != nil {
		t.Fatal(b.Error)
	}
	kinds := map[string]int{}
	for _, e := range b.Events {
		kinds[e.Type]++
	}
	if kinds["started"] != 1 || kinds["level"] < 3 || kinds["paused"] != 1 || kinds["resumed"] != 1 {
		t.Fatalf("events: %v", kinds)
	}
	var v struct {
		Seconds float64 `json:"seconds"`
		Tracks  []struct {
			ID      string  `json:"id"`
			Path    string  `json:"path"`
			Seconds float64 `json:"seconds"`
		} `json:"tracks"`
	}
	_ = json.Unmarshal([]byte(mustJSON(b.Value)), &v)
	// about 0.6 s recorded (the pause left out)
	if v.Seconds < 0.45 || v.Seconds > 0.9 || len(v.Tracks) != 2 {
		t.Fatalf("result: %s", mustJSON(b.Value))
	}
	for _, tr := range v.Tracks {
		rate, ch, off, n, err := readWAV(tr.Path)
		if err != nil || rate != RecordRate || ch != 1 || off != 44 {
			t.Fatalf("%s: %d %d %d %v", tr.ID, rate, ch, off, err)
		}
		sec := float64(n) / 2 / RecordRate
		if sec < 0.35 || sec > 0.9 || math.Abs(sec-tr.Seconds) > 0.01 {
			t.Fatalf("%s: %.3f s in the file, %.3f said", tr.ID, sec, tr.Seconds)
		}
		// the tone is there, at about half of full scale
		raw, _ := os.ReadFile(tr.Path)
		peak := 0
		for i := 44; i+1 < len(raw); i += 2 {
			if s := int(int16(binary.LittleEndian.Uint16(raw[i:]))); s > peak {
				peak = s
			}
		}
		if peak < 14000 || peak > 18000 {
			t.Fatalf("%s: peak %d", tr.ID, peak)
		}
	}
}

func TestRecordBadSource(t *testing.T) {
	env := testEnv(t)
	SetAudio(fakeAudio{rates: map[string]int{"mic": 16000}})
	defer SetAudio(osAudio())
	js := &Jobs{Env: env}
	j, _ := js.Start("audio.record", json.RawMessage(`{"dir":"~/rec2","tracks":[{"id":"me","source":"mic"},{"id":"them","source":"app:nope"}]}`))
	b := wait(t, js, j.ID)
	if b.Error == nil || b.Error.Code != CodeInvalid || !strings.Contains(b.Error.Message, "them") {
		t.Fatalf("want invalid_argument about track them: %+v", b.Error)
	}
	if _, err := os.Stat(env.Home + "/rec2/me.wav"); err == nil {
		t.Fatal("the files of a recording that did not start are left")
	}
}

func TestResampler(t *testing.T) {
	for _, rate := range []int{48000, 44100, 8000, 16000} {
		r := resampler{to: 16000}
		n := 0
		for i := 0; i < 100; i++ {
			n += len(r.push(make([]float32, rate/100), rate))
		}
		if math.Abs(float64(n-16000)) > 2 {
			t.Fatalf("%d Hz: one second gave %d samples", rate, n)
		}
	}
	// down: a constant stays constant
	r := resampler{to: 16000}
	in := make([]float32, 4800)
	for i := range in {
		in[i] = 0.25
	}
	for _, v := range r.push(in, 48000) {
		if math.Abs(float64(v-0.25)) > 1e-6 {
			t.Fatal(v)
		}
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
