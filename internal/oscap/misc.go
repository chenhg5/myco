package oscap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func init() {
	register(
		&Cap{Name: "clipboard.read", About: "读剪贴板里的文字", Missing: clipboardMissing, Fn: clipboardRead},
		&Cap{Name: "clipboard.write", About: "把文字放进剪贴板", Missing: clipboardMissing, Fn: clipboardWrite},
		&Cap{Name: "notify", About: "发一条系统通知", Missing: notifyMissing, Fn: notifyFn},
		&Cap{Name: "screen.capture", About: "截屏：整个屏幕、某个显示器或一块区域", Missing: screenMissing, Fn: screenCapture},
		&Cap{Name: "net.fetch", About: "从 App 发 HTTP 请求（网页自己不能上网）", Fn: netFetch},
	)
}

// runOut runs a helper program and returns its output (stderr in the error).
func runOut(ctx context.Context, env *Env, stdin []byte, extraEnv map[string]string, argv ...string) ([]byte, error) {
	return runOutFor(ctx, env, 30*time.Second, stdin, extraEnv, argv...)
}

// runOutFor is runOut with its own time limit (a dialog waits for the person).
func runOutFor(ctx context.Context, env *Env, limit time.Duration, stdin []byte, extraEnv map[string]string, argv ...string) ([]byte, error) {
	prog, err := env.LookPath(argv[0])
	if err != nil {
		return nil, errf(CodeUnsupported, "%s is not installed", argv[0])
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, prog, argv[1:]...)
	cmd.Env = env.Environ(extraEnv)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	hideWindow(cmd)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, errf(CodeFailed, "%s: %s", filepath.Base(argv[0]), msg)
	}
	return out, nil
}

func clipboardRead(ctx context.Context, env *Env, _ json.RawMessage, _ Emit) (any, error) {
	s, err := clipboardGet(ctx, env)
	if err != nil {
		return nil, err
	}
	return map[string]string{"text": s}, nil
}

func clipboardWrite(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	return true, clipboardSet(ctx, env, in.Text)
}

func notifyFn(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Subtitle string `json:"subtitle"`
		Sound    bool   `json:"sound"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Body) == "" {
		return nil, errf(CodeInvalid, "a title or a body is needed")
	}
	return true, notify(ctx, env, in.Title, in.Subtitle, in.Body, in.Sound)
}

// Region is a part of the screen, in points (macOS) or pixels.
type Region struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func screenCapture(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		// Display: 1 is the main one; 0 is all of them (macOS: the main one).
		Display int     `json:"display"`
		Region  *Region `json:"region"`
		// Format: png (default) or jpeg; Quality for jpeg (default 85).
		Format  string `json:"format"`
		Quality int    `json:"quality"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	if in.Region != nil && (in.Region.Width <= 0 || in.Region.Height <= 0) {
		return nil, errf(CodeInvalid, "a region needs a width and a height")
	}
	dir, err := os.MkdirTemp("", "myco-shot-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "shot.png")
	cropped, err := capture(ctx, env, file, in.Display, in.Region)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, errf(CodeFailed, "the screenshot was not made: %v", err)
	}
	needCrop := in.Region != nil && !cropped
	if needCrop || in.Format == "jpeg" || in.Format == "jpg" {
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, errf(CodeFailed, "the screenshot is not a PNG: %v", err)
		}
		if needCrop {
			r := image.Rect(in.Region.X, in.Region.Y, in.Region.X+in.Region.Width, in.Region.Y+in.Region.Height).Add(img.Bounds().Min).Intersect(img.Bounds())
			if r.Empty() {
				return nil, errf(CodeInvalid, "the region is outside the screen")
			}
			c := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
			draw.Draw(c, c.Bounds(), img, r.Min, draw.Src)
			img = c
		}
		var buf bytes.Buffer
		if in.Format == "jpeg" || in.Format == "jpg" {
			err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: clamp(in.Quality, 85, 1, 100)})
		} else {
			err = png.Encode(&buf, img)
		}
		if err != nil {
			return nil, err
		}
		b = buf.Bytes()
	}
	cfg, kind, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, errf(CodeFailed, "the screenshot cannot be read: %v", err)
	}
	return map[string]any{"type": "image/" + kind, "width": cfg.Width, "height": cfg.Height, "bytes": len(b), "base64": base64.StdEncoding.EncodeToString(b)}, nil
}

// The most net.fetch reads of a response by default, and at most.
const (
	fetchDefault = 16 << 20
	fetchMax     = 64 << 20
)

func netFetch(ctx context.Context, _ *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		URL      string            `json:"url"`
		Method   string            `json:"method"`
		Headers  map[string]string `json:"headers"`
		Body     string            `json:"body"`
		Encoding string            `json:"body_encoding"` // utf8 (default) or base64
		// Response: "auto" (default: text when it is text), "text" or "base64".
		Response  string `json:"response"`
		TimeoutMs int64  `json:"timeout_ms"`
		MaxBytes  int64  `json:"max_bytes"`
		// Redirect: follow redirects (default true).
		Redirect *bool `json:"redirect"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		return nil, errf(CodeInvalid, "the url must start with http:// or https://")
	}
	timeout := time.Duration(in.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	max := in.MaxBytes
	if max <= 0 {
		max = fetchDefault
	}
	if max > fetchMax {
		max = fetchMax
	}
	body, err := content(in.Body, in.Encoding)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var rd io.Reader
	if len(body) > 0 {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, in.URL, rd)
	if err != nil {
		return nil, errf(CodeInvalid, "%v", err)
	}
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "myco-app")
	}
	client := &http.Client{}
	if in.Redirect != nil && !*in.Redirect {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errf(CodeTimeout, "no answer from %s in %s", req.URL.Host, timeout)
		}
		return nil, errf(CodeFailed, "%v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, errf(CodeFailed, "reading the answer: %v", err)
	}
	truncated := int64(len(b)) > max
	if truncated {
		b = b[:max]
	}
	headers := map[string]string{}
	for k, v := range resp.Header {
		headers[strings.ToLower(k)] = strings.Join(v, ", ")
	}
	out := map[string]any{"status": resp.StatusCode, "status_text": http.StatusText(resp.StatusCode), "url": resp.Request.URL.String(), "headers": headers}
	if truncated {
		out["truncated"] = true
	}
	asText := in.Response == "text" || in.Response != "base64" && isText(resp.Header.Get("Content-Type")) && utf8.Valid(b)
	if asText {
		out["body"], out["encoding"] = string(b), "utf8"
	} else {
		out["body"], out["encoding"] = base64.StdEncoding.EncodeToString(b), "base64"
	}
	return out, nil
}

func isText(ct string) bool {
	mt, _, _ := mime.ParseMediaType(ct)
	if mt == "" {
		return true
	}
	return strings.HasPrefix(mt, "text/") || strings.HasSuffix(mt, "json") || strings.HasSuffix(mt, "xml") ||
		mt == "application/javascript" || mt == "application/x-www-form-urlencoded" || strings.HasSuffix(mt, "+yaml") || mt == "application/yaml"
}
