package oscap

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// Linux: PulseAudio, or PipeWire with its PulseAudio server (most desktops):
// parec records the default microphone, the default output's monitor (what
// the computer plays) or one app's stream (--monitor-stream).

type pulseAudio struct{}

func osAudio() AudioBackend {
	if _, err := exec.LookPath("parec"); err != nil {
		return nil
	}
	return pulseAudio{}
}

func (pulseAudio) Sources(ctx context.Context) ([]AudioSource, error) {
	out := []AudioSource{
		{ID: "mic", Kind: "mic", Name: "默认麦克风", Default: true},
		{ID: "system", Kind: "system", Name: "电脑的声音"},
	}
	b, err := exec.CommandContext(ctx, "pactl", "list", "sink-inputs").Output()
	if err != nil {
		return out, nil
	}
	return append(out, sinkInputs(string(b))...), nil
}

// sinkInputs reads `pactl list sink-inputs`: one source per app playing.
func sinkInputs(s string) []AudioSource {
	var out []AudioSource
	var cur *AudioSource
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Sink Input #") {
			out = append(out, AudioSource{ID: "app:" + strings.TrimPrefix(t, "Sink Input #"), Kind: "app", Playing: true})
			cur = &out[len(out)-1]
			continue
		}
		if cur == nil {
			continue
		}
		if t == "Corked: yes" {
			cur.Playing = false
		}
		k, v, ok := strings.Cut(t, " = ")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		switch k {
		case "application.name":
			cur.Name = v
		case "application.process.binary":
			cur.Bundle = v
		}
	}
	for i := range out {
		if out[i].Name == "" {
			out[i].Name = out[i].Bundle
		}
	}
	return out
}

func (pulseAudio) Capture(ctx context.Context, id string, f func([]float32, int)) (func(), error) {
	argv := []string{"parec", "--raw", "--format=float32le", "--rate=" + strconv.Itoa(RecordRate), "--channels=1", "--latency-msec=50"}
	switch {
	case id == "mic":
		argv = append(argv, "--device=@DEFAULT_SOURCE@")
	case id == "system":
		argv = append(argv, "--device=@DEFAULT_MONITOR@")
	case strings.HasPrefix(id, "app:"):
		n := strings.TrimPrefix(id, "app:")
		if _, err := strconv.Atoi(n); err != nil {
			return nil, errf(CodeInvalid, "no such source: %s (see audio.sources)", id)
		}
		argv = append(argv, "--monitor-stream="+n)
	default:
		return nil, errf(CodeInvalid, "no such source: %s (see audio.sources)", id)
	}
	cctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r := bufio.NewReaderSize(out, 1<<16)
		buf := make([]byte, 4*RecordRate/20)
		for {
			n, err := io.ReadFull(r, buf)
			if n >= 4 {
				s := make([]float32, n/4)
				for i := range s {
					s[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
				}
				f(s, RecordRate)
			}
			if err != nil {
				return
			}
		}
	}()
	return func() { cancel(); _ = cmd.Wait(); wg.Wait() }, nil
}
