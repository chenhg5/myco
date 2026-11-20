package oscap

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Recording sound: a microphone, the computer's own sound (what a meeting
// app plays), or one app's. Each source is a track, written to its own WAV
// file (16 kHz, mono, 16-bit: what speech recognition reads), so "me" (the
// microphone) and "them" (the computer's sound) stay apart.
//
// What records on each system is an AudioBackend: Linux has one here
// (PulseAudio / PipeWire, audio_linux.go); the Mac's needs the App (Core
// Audio, through cgo), which sets it with SetAudio; Windows' is not there
// yet.

// AudioSource is something to record.
type AudioSource struct {
	// ID is what audio.record takes: "mic", "system", "app:<id>".
	ID string `json:"id"`
	// Kind: mic, system or app.
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Default: the system's default microphone.
	Default bool `json:"default,omitempty"`
	// Playing: the app is making sound now.
	Playing bool `json:"playing,omitempty"`
	// Bundle (macOS) or program name of an app.
	Bundle string `json:"bundle,omitempty"`
}

// AudioBackend records sound on this computer.
type AudioBackend interface {
	// Sources are what can be recorded now.
	Sources(ctx context.Context) ([]AudioSource, error)
	// Capture starts recording a source: f gets its sound (mono, at rate)
	// until stop is called. f is called from one goroutine at a time.
	Capture(ctx context.Context, id string, f func(samples []float32, rate int)) (stop func(), err error)
}

var (
	audioMu   sync.Mutex
	audioBack AudioBackend = osAudio()
)

// SetAudio sets what records sound (the Mac App sets its own).
func SetAudio(b AudioBackend) {
	audioMu.Lock()
	audioBack = b
	audioMu.Unlock()
}

func audioBackend() AudioBackend {
	audioMu.Lock()
	defer audioMu.Unlock()
	return audioBack
}

func audioMissing(*Env) string {
	if audioBackend() != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return "recording sound needs the myco App on this Mac"
	case "linux":
		return "recording sound needs PulseAudio or PipeWire (parec, from pulseaudio-utils)"
	}
	return "recording sound is not available on " + osName() + " yet"
}

func init() {
	register(
		&Cap{Name: "audio.sources", About: "能录的声音：麦克风、电脑的声音、正在出声的应用", Missing: audioMissing, Fn: audioSources},
		&Cap{Name: "audio.record", Job: true, About: "录音：每个来源一条音轨，存成 WAV（带音量）", Missing: audioMissing, Fn: audioRecord},
	)
}

func audioSources(ctx context.Context, _ *Env, _ json.RawMessage, _ Emit) (any, error) {
	b := audioBackend()
	if b == nil {
		return nil, unsupported("audio.sources")
	}
	list, err := b.Sources(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"sources": list}, nil
}

// RecordRate is the sample rate of recordings.
const RecordRate = 16000

// The longest a recording runs (then it stops by itself).
const maxRecord = 6 * time.Hour

type trackArg struct {
	// ID names the track ("me", "them"); its file is <id>.wav.
	ID     string `json:"id"`
	Source string `json:"source"`
}

type track struct {
	trackArg
	path    string
	w       *wavWriter
	rs      resampler
	stop    func()
	err     error
	started bool
	// the level since the last event
	sum   float64
	n     int
	peak  float32
	total int64 // samples written
}

func audioRecord(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error) {
	var in struct {
		Tracks []trackArg `json:"tracks"`
		// Dir: where the files go (default ~/.myco/recordings/<time>).
		Dir string `json:"dir"`
		// MaxMinutes: stop by itself after this long.
		MaxMinutes float64 `json:"max_minutes"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	b := audioBackend()
	if b == nil {
		return nil, unsupported("audio.record")
	}
	if len(in.Tracks) == 0 {
		in.Tracks = []trackArg{{ID: "me", Source: "mic"}, {ID: "them", Source: "system"}}
	}
	if len(in.Tracks) > 4 {
		return nil, errf(CodeInvalid, "at most 4 tracks")
	}
	seen := map[string]bool{}
	for i, t := range in.Tracks {
		if t.ID == "" {
			in.Tracks[i].ID = fmt.Sprintf("track%d", i+1)
		}
		id := in.Tracks[i].ID
		if strings.ContainsAny(id, `/\.:`) || seen[id] {
			return nil, errf(CodeInvalid, "track id %q: unique, without / \\ . :", id)
		}
		seen[id] = true
		if t.Source == "" {
			return nil, errf(CodeInvalid, "track %s: a source is needed (mic, system, app:…; see audio.sources)", id)
		}
	}
	dir := in.Dir
	if dir == "" {
		dir = filepath.Join(env.Home, ".myco", "recordings", time.Now().Format("20060102-150405"))
	}
	dir, err := env.abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	limit := maxRecord
	if in.MaxMinutes > 0 && time.Duration(in.MaxMinutes*float64(time.Minute)) < limit {
		limit = time.Duration(in.MaxMinutes * float64(time.Minute))
	}

	var mu sync.Mutex
	paused := false
	start := time.Now()
	var pausedFor time.Duration
	var pausedAt time.Time
	// the recording's own clock: seconds recorded, pauses left out (mu held)
	elapsed := func() time.Duration {
		d := time.Since(start) - pausedFor
		if paused {
			d -= time.Since(pausedAt)
		}
		return d
	}

	tracks := make([]*track, len(in.Tracks))
	closeAll := func() {
		for _, t := range tracks {
			if t == nil {
				continue
			}
			if t.stop != nil {
				t.stop()
			}
			mu.Lock()
			if t.w != nil {
				if err := t.w.Close(); err != nil && t.err == nil {
					t.err = err
				}
			}
			mu.Unlock()
		}
	}
	for i, a := range in.Tracks {
		p := filepath.Join(dir, a.ID+".wav")
		w, err := newWAV(p, RecordRate)
		if err != nil {
			closeAll()
			return nil, err
		}
		t := &track{trackArg: a, path: p, w: w, rs: resampler{to: RecordRate}}
		tracks[i] = t
		stop, err := b.Capture(ctx, a.Source, func(s []float32, rate int) {
			mu.Lock()
			defer mu.Unlock()
			if paused || t.w == nil {
				return
			}
			if !t.started {
				// (a source that starts late is padded, so the tracks stay in step)
				t.started = true
				if lag := int64(elapsed().Seconds() * RecordRate); lag > RecordRate/5 {
					t.err = t.w.Write(make([]float32, lag))
					t.total += lag
				}
			}
			out := t.rs.push(s, rate)
			for _, v := range s {
				a := float32(math.Abs(float64(v)))
				if a > t.peak {
					t.peak = a
				}
				t.sum += float64(v) * float64(v)
				t.n++
			}
			if len(out) > 0 {
				if err := t.w.Write(out); err != nil && t.err == nil {
					t.err = err
				}
				t.total += int64(len(out))
			}
		})
		if err != nil {
			closeAll()
			for _, t := range tracks {
				if t != nil {
					os.Remove(t.path)
				}
			}
			e := AsError(err)
			e.Message = fmt.Sprintf("track %s (%s): %s", a.ID, a.Source, e.Message)
			return nil, e
		}
		t.stop = stop
	}
	files := map[string]string{}
	for _, t := range tracks {
		files[t.ID] = t.path
	}
	emit("started", map[string]any{"dir": dir, "files": files, "rate": RecordRate})

	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	flush := time.NewTicker(5 * time.Second)
	defer flush.Stop()
	input := inputOf(ctx)
	why := "stopped"
loop:
	for {
		select {
		case <-ctx.Done():
			why = "cancelled"
			break loop
		case b, ok := <-input:
			if !ok {
				continue
			}
			switch strings.TrimSpace(string(b)) {
			case "pause":
				mu.Lock()
				if !paused {
					paused, pausedAt = true, time.Now()
				}
				sec := elapsed()
				mu.Unlock()
				emit("paused", map[string]any{"seconds": round3(sec.Seconds())})
			case "resume":
				mu.Lock()
				if paused {
					paused = false
					pausedFor += time.Since(pausedAt)
				}
				sec := elapsed()
				mu.Unlock()
				emit("resumed", map[string]any{"seconds": round3(sec.Seconds())})
			case "stop", "":
				break loop
			}
		case <-flush.C:
			mu.Lock()
			for _, t := range tracks {
				_ = t.w.Sync()
			}
			mu.Unlock()
		case <-tick.C:
			lv := map[string]any{}
			mu.Lock()
			for _, t := range tracks {
				rms := 0.0
				if t.n > 0 {
					rms = math.Sqrt(t.sum / float64(t.n))
				}
				lv[t.ID] = map[string]any{"rms": round3(rms), "peak": round3(float64(t.peak))}
				t.sum, t.n, t.peak = 0, 0, 0
			}
			sec := elapsed()
			mu.Unlock()
			emit("level", map[string]any{"seconds": round3(sec.Seconds()), "tracks": lv})
			if sec >= limit {
				why = "limit"
				break loop
			}
		}
	}
	mu.Lock()
	total := elapsed()
	mu.Unlock()
	closeAll()
	out := []map[string]any{}
	for _, t := range tracks {
		m := map[string]any{"id": t.ID, "source": t.Source, "path": t.path, "seconds": round3(float64(t.total) / RecordRate)}
		if t.err != nil {
			m["error"] = t.err.Error()
		}
		if fi, err := os.Stat(t.path); err == nil {
			m["bytes"] = fi.Size()
		}
		out = append(out, m)
	}
	return map[string]any{"dir": dir, "seconds": round3(total.Seconds()), "rate": RecordRate, "tracks": out, "ended": why}, nil
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// resampler turns sound at any rate into sound at `to`: going down, each
// output sample is the mean of the input samples it covers (a box filter,
// enough against aliasing for speech); going up, it interpolates.
type resampler struct {
	to   int
	pos  float64 // going down: input samples into the current output one; up: place between last and next
	acc  float64
	cnt  int
	last float32
	have bool
}

func (r *resampler) push(in []float32, rate int) []float32 {
	if rate <= 0 || len(in) == 0 {
		return nil
	}
	if rate == r.to {
		return append([]float32(nil), in...)
	}
	step := float64(rate) / float64(r.to)
	out := make([]float32, 0, int(float64(len(in))/step)+2)
	if step > 1 {
		for _, v := range in {
			r.acc += float64(v)
			r.cnt++
			r.pos++
			if r.pos >= step {
				out = append(out, float32(r.acc/float64(r.cnt)))
				r.pos -= step
				r.acc, r.cnt = 0, 0
			}
		}
		return out
	}
	// up: output samples every `step` input samples, between last and the next
	for _, v := range in {
		if !r.have {
			r.last, r.have = v, true
			out = append(out, v)
			continue
		}
		for r.pos+step <= 1 {
			r.pos += step
			out = append(out, r.last+(v-r.last)*float32(r.pos))
		}
		r.pos -= 1
		r.last = v
	}
	return out
}

// wavWriter writes a 16-bit mono WAV, its header kept right as it grows.
type wavWriter struct {
	f    *os.File
	rate int
	n    int64 // data bytes
	buf  []byte
}

func newWAV(path string, rate int) (*wavWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	w := &wavWriter{f: f, rate: rate}
	if _, err := f.Write(wavHeader(rate, 0)); err != nil {
		f.Close()
		return nil, err
	}
	return w, nil
}

func wavHeader(rate int, data int64) []byte {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+data))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 1) // mono
	binary.LittleEndian.PutUint32(h[24:], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(data))
	return h
}

func (w *wavWriter) Write(s []float32) error {
	if cap(w.buf) < len(s)*2 {
		w.buf = make([]byte, len(s)*2)
	}
	b := w.buf[:len(s)*2]
	for i, v := range s {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		binary.LittleEndian.PutUint16(b[i*2:], uint16(int16(v*32767)))
	}
	n, err := w.f.Write(b)
	w.n += int64(n)
	return err
}

// Sync puts the sizes in the header (so the file plays even if the App stops).
func (w *wavWriter) Sync() error {
	if _, err := w.f.WriteAt(wavHeader(w.rate, w.n)[:44], 0); err != nil {
		return err
	}
	return nil
}

func (w *wavWriter) Close() error {
	err := w.Sync()
	if e := w.f.Close(); err == nil {
		err = e
	}
	return err
}

// readWAV reads a 16-bit PCM WAV's format and where its samples are.
func readWAV(path string) (rate, channels int, dataOff, dataLen int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer f.Close()
	h := make([]byte, 12)
	if _, err := f.ReadAt(h, 0); err != nil || string(h[0:4]) != "RIFF" || string(h[8:12]) != "WAVE" {
		return 0, 0, 0, 0, errf(CodeInvalid, "%s is not a WAV file", path)
	}
	off := int64(12)
	bits := 0
	for {
		ch := make([]byte, 8)
		if _, err := f.ReadAt(ch, off); err != nil {
			return 0, 0, 0, 0, errf(CodeInvalid, "%s: no audio data", path)
		}
		size := int64(binary.LittleEndian.Uint32(ch[4:]))
		switch string(ch[0:4]) {
		case "fmt ":
			b := make([]byte, 16)
			if _, err := f.ReadAt(b, off+8); err != nil {
				return 0, 0, 0, 0, errf(CodeInvalid, "%s: bad format", path)
			}
			if fmtTag := binary.LittleEndian.Uint16(b[0:]); fmtTag != 1 && fmtTag != 0xFFFE {
				return 0, 0, 0, 0, errf(CodeInvalid, "%s is not PCM", path)
			}
			channels = int(binary.LittleEndian.Uint16(b[2:]))
			rate = int(binary.LittleEndian.Uint32(b[4:]))
			bits = int(binary.LittleEndian.Uint16(b[14:]))
		case "data":
			if bits != 16 || rate == 0 {
				return 0, 0, 0, 0, errf(CodeInvalid, "%s: only 16-bit PCM WAV is read", path)
			}
			fi, _ := f.Stat()
			if fi != nil && (size == 0 || off+8+size > fi.Size()) {
				// (a recording cut short: its header was not written at the end)
				size = fi.Size() - off - 8
			}
			return rate, channels, off + 8, size, nil
		}
		off += 8 + size + size%2
	}
}
