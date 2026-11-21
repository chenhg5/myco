package oscap

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Speech to text, on this computer: sherpa-onnx (k2-fsa, Apache-2.0), its
// own programs as it releases them, and a model (SenseVoice: Chinese,
// English, Cantonese, Japanese, Korean, with punctuation). Neither is part
// of myco: speech.install downloads them into ~/.myco/speech the first time
// (about 200 MB), and speech.transcribe runs them on a recording.
//
// MYCO_SPEECH_MIRROR replaces https://github.com in the download addresses
// (for a mirror that keeps the same paths).

const sherpaVersion = "v2.0.2"

type speechModel struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Languages []string `json:"languages"`
	// MB to download.
	Size int `json:"size_mb"`
	// where in releases/download/
	archive string
	dir     string
}

var speechModels = []speechModel{
	{ID: "sensevoice", Name: "SenseVoice（快，中英粤日韩）", Languages: []string{"zh", "en", "yue", "ja", "ko"}, Size: 158,
		archive: "asr-models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2025-09-09.tar.bz2", dir: "sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2025-09-09"},
}

const vadFile = "asr-models/silero_vad.onnx"

// Telling speakers apart (speech.diarize): pyannote's segmentation and a
// 3D-Speaker voice model (CAM++), about 35 MB together.
const (
	segArchive = "speaker-segmentation-models/sherpa-onnx-pyannote-segmentation-3-0.tar.bz2"
	segDir     = "sherpa-onnx-pyannote-segmentation-3-0"
	voiceFile  = "speaker-recongition-models/3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx"
	diarizeMB  = 35
)

func diarizeReady(env *Env) bool {
	m := filepath.Join(speechDir(env), "models")
	return exists(filepath.Join(m, segDir, "model.onnx")) && exists(filepath.Join(m, filepath.Base(voiceFile))) &&
		exists(engineProgram(env, "sherpa-onnx-offline-speaker-diarization"))
}

func speechModelOf(id string) (speechModel, bool) {
	if id == "" {
		return speechModels[0], true
	}
	for _, m := range speechModels {
		if m.ID == id {
			return m, true
		}
	}
	return speechModel{}, false
}

// engineArchive is sherpa-onnx's programs for this computer ("" when it has none).
func engineArchive() string {
	plat := ""
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		plat = "osx-arm64-shared-no-tts"
	case "darwin/amd64":
		plat = "osx-x64-shared-no-tts"
	case "linux/amd64":
		plat = "linux-x64-shared-no-tts"
	case "windows/amd64":
		plat = "win-x64-shared-MT-Release-no-tts"
	default:
		return ""
	}
	return fmt.Sprintf("sherpa-onnx-%s-%s", sherpaVersion, plat)
}

func speechDir(env *Env) string { return filepath.Join(env.Home, ".myco", "speech") }

func engineDir(env *Env) string { return filepath.Join(speechDir(env), "engine", engineArchive()) }

func engineProgram(env *Env, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(engineDir(env), "bin", name)
}

func speechMissing(*Env) string {
	if engineArchive() == "" {
		return "speech to text is not available on " + runtime.GOOS + "/" + runtime.GOARCH
	}
	return ""
}

func init() {
	register(
		&Cap{Name: "speech.status", About: "本机转写：引擎和模型装好了没有", Missing: speechMissing, Fn: speechStatus},
		&Cap{Name: "speech.install", Job: true, About: "下载转写引擎和模型（带进度）", Missing: speechMissing, Fn: speechInstall},
		&Cap{Name: "speech.transcribe", Job: true, About: "把录音转成文字：一句一段，带时间（边转边出）", Missing: speechMissing, Fn: speechTranscribe},
		&Cap{Name: "speech.diarize", Job: true, About: "分出录音里的不同说话人：谁在什么时候说话", Missing: speechMissing, Fn: speechDiarize},
	)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func modelReady(env *Env, m speechModel) bool {
	d := filepath.Join(speechDir(env), "models", m.dir)
	return exists(filepath.Join(d, "tokens.txt")) && (exists(filepath.Join(d, "model.int8.onnx")) || exists(filepath.Join(d, "model.onnx")))
}

func speechStatus(_ context.Context, env *Env, _ json.RawMessage, _ Emit) (any, error) {
	engine := exists(engineProgram(env, "sherpa-onnx-vad-with-offline-asr"))
	vad := exists(filepath.Join(speechDir(env), "models", "silero_vad.onnx"))
	models := []map[string]any{}
	ready := false
	for _, m := range speechModels {
		ok := modelReady(env, m)
		ready = ready || ok
		models = append(models, map[string]any{"id": m.ID, "name": m.Name, "languages": m.Languages, "size_mb": m.Size, "installed": ok})
	}
	size := 0
	if !engine {
		size += 25
	}
	diarize := diarizeReady(env)
	if !diarize {
		size += diarizeMB
	}
	return map[string]any{
		"ready": engine && vad && ready, "engine": engine, "engine_version": sherpaVersion, "vad": vad,
		"models": models, "dir": speechDir(env), "engine_size_mb": size, "diarize": diarize,
	}, nil
}

func releaseURL(path string) string {
	base := "https://github.com"
	if m := strings.TrimRight(os.Getenv("MYCO_SPEECH_MIRROR"), "/"); m != "" {
		base = m
	}
	return base + "/k2-fsa/sherpa-onnx/releases/download/" + path
}

func speechInstall(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error) {
	var in struct {
		Model string `json:"model"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	m, ok := speechModelOf(in.Model)
	if !ok {
		return nil, errf(CodeInvalid, "no such model: %s (see speech.status)", in.Model)
	}
	dir := speechDir(env)
	if err := os.MkdirAll(filepath.Join(dir, "models"), 0o755); err != nil {
		return nil, err
	}
	if !exists(engineProgram(env, "sherpa-onnx-vad-with-offline-asr")) {
		a := engineArchive()
		if err := fetchArchive(ctx, emit, "engine", releaseURL(sherpaVersion+"/"+a+".tar.bz2"), filepath.Join(dir, "engine")); err != nil {
			return nil, err
		}
	}
	vad := filepath.Join(dir, "models", "silero_vad.onnx")
	if !exists(vad) {
		if err := fetchFile(ctx, emit, "vad", releaseURL(vadFile), vad); err != nil {
			return nil, err
		}
	}
	if !modelReady(env, m) {
		if err := fetchArchive(ctx, emit, "model", releaseURL(m.archive), filepath.Join(dir, "models")); err != nil {
			return nil, err
		}
		// (the model's own test recordings are not needed)
		os.RemoveAll(filepath.Join(dir, "models", m.dir, "test_wavs"))
	}
	if !exists(filepath.Join(dir, "models", segDir, "model.onnx")) {
		if err := fetchArchive(ctx, emit, "speakers", releaseURL(segArchive), filepath.Join(dir, "models")); err != nil {
			return nil, err
		}
	}
	if voice := filepath.Join(dir, "models", filepath.Base(voiceFile)); !exists(voice) {
		if err := fetchFile(ctx, emit, "voices", releaseURL(voiceFile), voice); err != nil {
			return nil, err
		}
	}
	return speechStatus(ctx, env, nil, emit)
}

// download fetches a URL to w, sending progress.
func download(ctx context.Context, emit Emit, step, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	var resp *http.Response
	for _, c := range downloadClients() {
		if resp, err = c.Do(req); err == nil || ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		return errf(CodeFailed, "download %s: %v (set a proxy in the App's settings if GitHub is slow or blocked here)", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errf(CodeFailed, "download %s: %s", url, resp.Status)
	}
	total := resp.ContentLength
	var got int64
	last := time.Time{}
	buf := make([]byte, 256<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			got += int64(n)
			if time.Since(last) > 300*time.Millisecond {
				emit("progress", map[string]any{"step": step, "received": got, "total": total})
				last = time.Now()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return errf(CodeFailed, "download %s: %v", url, err)
		}
	}
	if total > 0 && got != total {
		return errf(CodeFailed, "download %s: got %d of %d bytes", url, got, total)
	}
	emit("progress", map[string]any{"step": step, "received": got, "total": total, "done": true})
	return nil
}

func fetchFile(ctx context.Context, emit Emit, step, url, dst string) error {
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = download(ctx, emit, step, url, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// fetchArchive downloads a .tar.bz2 and unpacks it into dir (into a
// temporary folder first, moved in place when whole).
func fetchArchive(ctx context.Context, emit Emit, step, url, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = download(ctx, emit, step, url, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	emit("progress", map[string]any{"step": step, "unpacking": true})
	stage, err := os.MkdirTemp(dir, ".unpack-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := untarBz2(ctx, tmp.Name(), stage); err != nil {
		return err
	}
	ents, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	for _, e := range ents {
		dst := filepath.Join(dir, e.Name())
		os.RemoveAll(dst)
		if err := os.Rename(filepath.Join(stage, e.Name()), dst); err != nil {
			return err
		}
	}
	return nil
}

func untarBz2(ctx context.Context, src, dir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(bzip2.NewReader(bufio.NewReaderSize(f, 1<<20)))
	root := filepath.Clean(dir) + string(os.PathSeparator)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errf(CodeFailed, "unpack: %v", err)
		}
		p := filepath.Join(dir, filepath.FromSlash(h.Name))
		if !strings.HasPrefix(p, root) {
			return errf(CodeFailed, "unpack: %s is outside the archive", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			w, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, tr)
			if cerr := w.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			t := filepath.Join(filepath.Dir(p), filepath.FromSlash(h.Linkname))
			if !strings.HasPrefix(filepath.Clean(t), root) {
				continue
			}
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			_ = os.Remove(p)
			if err := os.Symlink(h.Linkname, p); err != nil {
				return err
			}
		}
	}
}

// ---- transcribing ----

// Segment is a sentence of a recording.
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// A long recording is transcribed in parts of about this long, cut where
// it is quietest, so its sentences come while it is being transcribed.
const speechPart = 4 * 60

var segLine = regexp.MustCompile(`^\s*(\d+(?:\.\d+)?)\s*--\s*(\d+(?:\.\d+)?)\s*:\s*(.*)$`)

func speechTranscribe(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error) {
	var in struct {
		Path     string `json:"path"`
		Model    string `json:"model"`
		Language string `json:"language"`
		// Offset: seconds to add to every time (a part of a longer recording).
		Offset float64 `json:"offset"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	m, ok := speechModelOf(in.Model)
	if !ok {
		return nil, errf(CodeInvalid, "no such model: %s (see speech.status)", in.Model)
	}
	prog := engineProgram(env, "sherpa-onnx-vad-with-offline-asr")
	vad := filepath.Join(speechDir(env), "models", "silero_vad.onnx")
	if !exists(prog) || !exists(vad) || !modelReady(env, m) {
		return nil, errf(CodeNotFound, "speech to text is not installed here: install it first (speech.install)")
	}
	work, err := os.MkdirTemp("", "myco-speech-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	wav, err := asWAV16k(ctx, env, p, work)
	if err != nil {
		return nil, err
	}
	samples, err := readPCM16(wav)
	if err != nil {
		return nil, err
	}
	total := float64(len(samples)) / RecordRate
	lang := in.Language
	if lang == "" {
		lang = "auto"
	}
	threads := runtime.NumCPU() / 2
	if threads < 2 {
		threads = 2
	} else if threads > 6 {
		threads = 6
	}
	md := filepath.Join(speechDir(env), "models", m.dir)
	model := filepath.Join(md, "model.int8.onnx")
	if !exists(model) {
		model = filepath.Join(md, "model.onnx")
	}
	start := time.Now()
	var all []Segment
	emit("progress", map[string]any{"done": 0, "total": round3(total)})
	for _, part := range splitQuiet(samples, speechPart*RecordRate) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		chunk := filepath.Join(work, "part.wav")
		if err := writePCM16(chunk, samples[part[0]:part[1]]); err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, prog,
			"--silero-vad-model="+vad, "--sense-voice-model="+model, "--tokens="+filepath.Join(md, "tokens.txt"),
			"--sense-voice-use-itn=1", "--sense-voice-language="+lang, "--num-threads="+strconv.Itoa(threads), chunk)
		cmd.Env = engineEnv(env)
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			msg := err.Error()
			if ee, ok := err.(*exec.ExitError); ok {
				msg = lastLines(string(ee.Stderr), 3)
			}
			return nil, errf(CodeFailed, "transcribe: %s", msg)
		}
		off := float64(part[0])/RecordRate + in.Offset
		for _, s := range parseSegments(string(out)) {
			s.Start, s.End = round3(s.Start+off), round3(s.End+off)
			all = append(all, s)
			emit("segment", s)
		}
		emit("progress", map[string]any{"done": round3(float64(part[1]) / RecordRate), "total": round3(total)})
	}
	if all == nil {
		all = []Segment{}
	}
	return map[string]any{"segments": all, "seconds": round3(total), "ms": time.Since(start).Milliseconds(), "model": m.ID}, nil
}

func parseSegments(out string) []Segment {
	var segs []Segment
	for _, line := range strings.Split(out, "\n") {
		mm := segLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if mm == nil {
			continue
		}
		text := strings.TrimSpace(mm[3])
		if text == "" {
			continue
		}
		a, _ := strconv.ParseFloat(mm[1], 64)
		b, _ := strconv.ParseFloat(mm[2], 64)
		segs = append(segs, Segment{Start: a, End: b, Text: text})
	}
	return segs
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

// engineEnv: the environment for sherpa-onnx's programs, finding their
// libraries next to them.
func engineEnv(env *Env) []string {
	lib := filepath.Join(engineDir(env), "lib")
	over := map[string]string{}
	switch runtime.GOOS {
	case "darwin":
		over["DYLD_LIBRARY_PATH"] = lib
	case "linux":
		over["LD_LIBRARY_PATH"] = lib + string(os.PathListSeparator) + os.Getenv("LD_LIBRARY_PATH")
	case "windows":
		over["PATH"] = lib + string(os.PathListSeparator) + filepath.Join(engineDir(env), "bin") + string(os.PathListSeparator) + env.PATH()
	}
	return env.Environ(over)
}

// splitQuiet cuts samples into parts of about n, each cut at the quietest
// moment (a 0.2 s window) within 15 s before where it would fall.
func splitQuiet(s []int16, n int) [][2]int {
	var parts [][2]int
	at := 0
	win := RecordRate / 5
	look := 15 * RecordRate
	for len(s)-at > n+n/4 {
		cut := at + n
		best, bestE := cut, math.MaxFloat64
		for c := cut - look; c+win <= cut; c += win / 2 {
			if c <= at {
				continue
			}
			e := 0.0
			for _, v := range s[c : c+win] {
				e += float64(v) * float64(v)
			}
			if e < bestE {
				best, bestE = c+win/2, e
			}
		}
		parts = append(parts, [2]int{at, best})
		at = best
	}
	return append(parts, [2]int{at, len(s)})
}

func readPCM16(path string) ([]int16, error) {
	rate, ch, off, n, err := readWAV(path)
	if err != nil {
		return nil, err
	}
	if rate != RecordRate || ch != 1 {
		return nil, errf(CodeInvalid, "%s: want 16 kHz mono", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := make([]byte, n)
	k, err := f.ReadAt(b, off)
	if err != nil && err != io.EOF {
		return nil, err
	}
	b = b[:k-k%2]
	s := make([]int16, len(b)/2)
	for i := range s {
		s[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return s, nil
}

func writePCM16(path string, s []int16) error {
	b := make([]byte, 44+len(s)*2)
	copy(b, wavHeader(RecordRate, int64(len(s)*2)))
	for i, v := range s {
		binary.LittleEndian.PutUint16(b[44+i*2:], uint16(v))
	}
	return os.WriteFile(path, b, 0o600)
}

// asWAV16k gives a 16 kHz mono WAV of a recording: itself when it is one;
// another WAV converted here; other formats through afconvert (macOS) or
// ffmpeg (when installed).
func asWAV16k(ctx context.Context, env *Env, p, work string) (string, error) {
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	rate, ch, off, n, err := readWAV(p)
	if err == nil {
		if rate == RecordRate && ch == 1 {
			return p, nil
		}
		return convertWAV(p, rate, ch, off, n, filepath.Join(work, "in.wav"))
	}
	out := filepath.Join(work, "in.wav")
	if runtime.GOOS == "darwin" {
		if _, err := runOutFor(ctx, env, 30*time.Minute, nil, nil, "afconvert", "-f", "WAVE", "-d", "LEI16@16000", "-c", "1", p, out); err == nil {
			return out, nil
		}
	}
	if ff, err := env.LookPath("ffmpeg"); err == nil {
		if _, err := runOutFor(ctx, env, 30*time.Minute, nil, nil, ff, "-loglevel", "error", "-y", "-i", p, "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", out); err == nil {
			return out, nil
		}
	}
	return "", errf(CodeUnsupported, "%s: only WAV can be read here (install ffmpeg for other formats)", filepath.Base(p))
}

func convertWAV(src string, rate, ch int, off, n int64, dst string) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w, err := newWAV(dst, RecordRate)
	if err != nil {
		return "", err
	}
	r := bufio.NewReaderSize(io.NewSectionReader(f, off, n), 1<<20)
	rs := resampler{to: RecordRate}
	frame := make([]byte, 2*ch)
	block := make([]float32, 0, 8192)
	for {
		if _, err := io.ReadFull(r, frame); err != nil {
			break
		}
		v := float32(0)
		for c := 0; c < ch; c++ {
			v += float32(int16(binary.LittleEndian.Uint16(frame[c*2:]))) / 32768
		}
		block = append(block, v/float32(ch))
		if len(block) == cap(block) {
			_ = w.Write(rs.push(block, rate))
			block = block[:0]
		}
	}
	if len(block) > 0 {
		_ = w.Write(rs.push(block, rate))
	}
	return dst, w.Close()
}

// ---- who speaks when ----

// Turn is a stretch of one speaker (numbered from 0, in the order they
// first speak).
type Turn struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker int     `json:"speaker"`
}

var (
	turnLine     = regexp.MustCompile(`^\s*(\d+(?:\.\d+)?)\s*--\s*(\d+(?:\.\d+)?)\s+speaker_(\d+)\s*$`)
	progressLine = regexp.MustCompile(`progress\s+(\d+(?:\.\d+)?)%`)
)

func speechDiarize(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error) {
	var in struct {
		Path string `json:"path"`
		// Speakers: how many there are, when known (0: found from the voices).
		Speakers int `json:"speakers"`
		// Threshold: how different voices must be to be two people (0.5 by default; larger, fewer).
		Threshold float64 `json:"threshold"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	p, err := env.abs(in.Path)
	if err != nil {
		return nil, err
	}
	if !diarizeReady(env) {
		return nil, errf(CodeNotFound, "telling speakers apart is not installed here: install it first (speech.install)")
	}
	if in.Speakers < 0 || in.Speakers > 20 {
		return nil, errf(CodeInvalid, "speakers: 0 (find them) to 20")
	}
	if in.Threshold <= 0 {
		in.Threshold = 0.5
	}
	work, err := os.MkdirTemp("", "myco-diarize-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	wav, err := asWAV16k(ctx, env, p, work)
	if err != nil {
		return nil, err
	}
	threads := runtime.NumCPU() / 2
	if threads < 2 {
		threads = 2
	} else if threads > 6 {
		threads = 6
	}
	md := filepath.Join(speechDir(env), "models")
	argv := []string{
		"--segmentation.pyannote-model=" + filepath.Join(md, segDir, "model.onnx"),
		"--embedding.model=" + filepath.Join(md, filepath.Base(voiceFile)),
		"--segmentation.num-threads=" + strconv.Itoa(threads), "--embedding.num-threads=" + strconv.Itoa(threads),
	}
	if in.Speakers > 0 {
		argv = append(argv, "--clustering.num-clusters="+strconv.Itoa(in.Speakers))
	} else {
		argv = append(argv, "--clustering.cluster-threshold="+strconv.FormatFloat(in.Threshold, 'f', 3, 64))
	}
	argv = append(argv, wav)
	start := time.Now()
	cmd := exec.CommandContext(ctx, engineProgram(env, "sherpa-onnx-offline-speaker-diarization"), argv...)
	cmd.Env = engineEnv(env)
	hideWindow(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var turns []Turn
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	last := time.Time{}
	for sc.Scan() {
		line := sc.Text()
		if m := progressLine.FindStringSubmatch(line); m != nil {
			if time.Since(last) > 300*time.Millisecond {
				v, _ := strconv.ParseFloat(m[1], 64)
				emit("progress", map[string]any{"percent": v})
				last = time.Now()
			}
			continue
		}
		if t, ok := parseTurn(line); ok {
			turns = append(turns, t)
		}
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errf(CodeFailed, "diarize: %s", lastLines(stderr.String(), 3))
	}
	turns = renumber(turns)
	n := 0
	for _, t := range turns {
		if t.Speaker+1 > n {
			n = t.Speaker + 1
		}
	}
	if turns == nil {
		turns = []Turn{}
	}
	return map[string]any{"turns": turns, "speakers": n, "ms": time.Since(start).Milliseconds()}, nil
}

func parseTurn(line string) (Turn, bool) {
	m := turnLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
	if m == nil {
		return Turn{}, false
	}
	a, _ := strconv.ParseFloat(m[1], 64)
	b, _ := strconv.ParseFloat(m[2], 64)
	k, _ := strconv.Atoi(m[3])
	return Turn{Start: round3(a), End: round3(b), Speaker: k}, true
}

// renumber sorts the turns by time and numbers the speakers 0, 1, 2… in
// the order they first speak.
func renumber(turns []Turn) []Turn {
	sort.Slice(turns, func(i, j int) bool { return turns[i].Start < turns[j].Start })
	ids := map[int]int{}
	for i := range turns {
		k, ok := ids[turns[i].Speaker]
		if !ok {
			k = len(ids)
			ids[turns[i].Speaker] = k
		}
		turns[i].Speaker = k
	}
	return turns
}
