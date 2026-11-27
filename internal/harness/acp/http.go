package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/harness"
)

// CodeBuddy advertises its actual endpoint on stderr (some builds ignore
// --port). Never discover a server by scanning ports or following log URLs
// outside loopback. Only the newly started child can advertise this endpoint.
var httpEndpoint = regexp.MustCompile(`http://(?:127\.0\.0\.1|localhost|\[::1\]):[0-9]+(?:/[^\s]*)?`)

const maxHTTPFrame = 4 << 20

type endpointWriter struct {
	mu      sync.Mutex
	partial []byte
	found   chan string
}

func (w *endpointWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Bound startup diagnostics without retaining configuration or credentials.
	w.partial = append(w.partial, p...)
	for {
		i := bytes.IndexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		line := w.partial[:i]
		for _, candidate := range httpEndpoint.FindAllString(string(line), -1) {
			if endpoint, err := announcedEndpoint(candidate); err == nil {
				select {
				case w.found <- endpoint:
				default:
				}
			}
		}
		w.partial = w.partial[i+1:]
	}
	if len(w.partial) > 8192 {
		w.partial = nil
	}
	return len(p), nil
}

// WorkBuddy announces /api/v1/acp, while some standalone builds announce
// the origin. Accept only those known forms, never arbitrary URL paths.
func announcedEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Path == "/api/v1/acp" {
		u.Path = ""
	}
	return localEndpoint(u.String())
}

func localEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Port() == "" {
		return "", errors.New("acp: invalid local HTTP endpoint")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("acp: invalid HTTP port")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", errors.New("acp: HTTP endpoint must be loopback")
	}
	// Avoid DNS resolution for localhost, even when a proxy is configured.
	if host == "localhost" {
		u.Host = net.JoinHostPort("127.0.0.1", u.Port())
	}
	u.Path = ""
	return u.String(), nil
}

type httpRPC struct {
	startOnce               sync.Once
	ctx                     context.Context
	cancel                  context.CancelFunc
	client                  *http.Client
	transport               *http.Transport
	base, connection, token string
	send                    chan []byte
	receive                 chan []byte
	c                       *conn
	diag                    *tailBuffer
	once                    sync.Once
	wg                      sync.WaitGroup
}

func newHTTPRPC(ctx, startup context.Context, endpoint string, diag *tailBuffer) (*httpRPC, error) {
	base, err := localEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}
	h := &httpRPC{ctx: ctx, cancel: cancel, transport: tr, base: base, diag: diag, send: make(chan []byte, 16), receive: make(chan []byte, 32)}
	h.client = &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	connectCtx, stop := context.WithTimeout(startup, 15*time.Second)
	defer stop()
	req, _ := http.NewRequestWithContext(connectCtx, http.MethodPost, base+"/api/v1/acp/connect", nil)
	h.headers(req)
	resp, err := h.client.Do(req)
	if err != nil {
		h.close()
		return nil, errors.New("acp: local HTTP connect failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		h.close()
		return nil, fmt.Errorf("acp: HTTP connect returned status %d", resp.StatusCode)
	}
	var connected struct {
		ConnectionID string `json:"connectionId"`
		SessionToken string `json:"sessionToken"`
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil || json.Unmarshal(b, &connected) != nil || connected.ConnectionID == "" || len(connected.ConnectionID) > 512 || len(connected.SessionToken) > 4096 || strings.ContainsAny(connected.ConnectionID+connected.SessionToken, "\r\n") {
		h.close()
		return nil, errors.New("acp: invalid HTTP connection response")
	}
	h.connection, h.token = connected.ConnectionID, connected.SessionToken
	h.c = newConn(h)

	return h, nil
}

func (h *httpRPC) start() {
	h.startOnce.Do(func() {
		// One dispatcher preserves the stdio callback ordering across POST and GET.
		h.wg.Go(func() {
			defer h.c.shutdown()
			for {
				select {
				case b := <-h.receive:
					h.c.dispatch(b)
				case <-h.ctx.Done():
					return
				}
			}
		})
		h.wg.Go(h.sender)
		h.wg.Go(func() { h.exchange(http.MethodGet, nil) })
	})
}

func (h *httpRPC) headers(req *http.Request) {
	req.Header.Set("x-codebuddy-request", "1")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if req.Method == http.MethodGet {
		req.Header.Set("Accept", "text/event-stream")
	}
	if h.connection != "" {
		req.Header.Set("acp-connection-id", h.connection)
	}
	if h.token != "" {
		req.Header.Set("acp-session-token", h.token)
	}
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
}

func (h *httpRPC) Write(p []byte) (int, error) {
	h.start()
	if len(p) > maxHTTPFrame {
		return 0, errors.New("acp: HTTP request is too large")
	}
	select {
	case h.send <- bytes.TrimSpace(bytes.Clone(p)):
		return len(p), nil
	case <-h.ctx.Done():
		return 0, errClosed
	}
}

func (h *httpRPC) sender() {
	slots := make(chan struct{}, 8)
	for {
		select {
		case <-h.ctx.Done():
			return
		case b := <-h.send:
			select {
			case slots <- struct{}{}:
			case <-h.ctx.Done():
				return
			}
			h.wg.Go(func() { defer func() { <-slots }(); h.exchange(http.MethodPost, b) })
		}
	}
}

func (h *httpRPC) fail(message string) {
	// Never retain response bodies, URLs, headers or connection credentials.
	_, _ = h.diag.Write([]byte(message))
	h.cancel()
}

func (h *httpRPC) exchange(method string, b []byte) {
	var body io.Reader
	if b != nil {
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(h.ctx, method, h.base+"/api/v1/acp", body)
	if err != nil {
		h.fail("ACP HTTP request failed")
		return
	}
	h.headers(req)
	resp, err := h.client.Do(req)
	if err != nil {
		if h.ctx.Err() == nil {
			h.fail("ACP HTTP connection failed")
		}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.fail(fmt.Sprintf("ACP HTTP returned status %d", resp.StatusCode))
		return
	}
	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent {
		return
	}
	var sent message
	_ = json.Unmarshal(b, &sent)
	expectReply := method == http.MethodPost && sent.Method != "" && len(sent.ID) > 0
	replied := false
	emit := func(b []byte) error {
		if !json.Valid(b) {
			return errors.New("invalid JSON")
		}
		var got message
		if expectReply && json.Unmarshal(b, &got) == nil && got.Method == "" && bytes.Equal(got.ID, sent.ID) {
			replied = true
		}
		select {
		case h.receive <- bytes.Clone(b):
			return nil
		case <-h.ctx.Done():
			return h.ctx.Err()
		}
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		err = readSSE(resp.Body, emit)
	} else {
		var frame []byte
		frame, err = io.ReadAll(io.LimitReader(resp.Body, maxHTTPFrame+1))
		if err == nil && len(frame) > maxHTTPFrame {
			err = errors.New("oversized response")
		}
		if err == nil {
			err = emit(frame)
		}
	}
	if err == nil && expectReply && !replied {
		err = errors.New("response ended before RPC reply")
	}
	if err != nil && h.ctx.Err() == nil {
		h.fail("ACP HTTP stream was invalid or interrupted")
	} else if method == http.MethodGet && h.ctx.Err() == nil {
		h.fail("ACP HTTP event stream closed")
	}
}

func readSSE(r io.Reader, emit func([]byte) error) error {
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 4096), maxHTTPFrame)
	var data []byte
	flush := func() error {
		if len(data) == 0 {
			return nil
		}
		b := data
		data = nil
		return emit(bytes.TrimSuffix(b, []byte{'\n'}))
	}
	for scan.Scan() {
		line := scan.Bytes()
		if len(line) == 0 {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			v := bytes.TrimPrefix(line, []byte("data:"))
			v = bytes.TrimPrefix(v, []byte{' '})
			if len(data)+len(v) > maxHTTPFrame {
				return errors.New("oversized SSE event")
			}
			data = append(data, v...)
			data = append(data, '\n')
		}
	}
	if scan.Err() != nil {
		return scan.Err()
	}
	return flush()
}

func (h *httpRPC) close() {
	h.once.Do(func() {
		if h.connection != "" && h.ctx.Err() == nil {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(h.ctx), time.Second)
			req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, h.base+"/api/v1/acp", nil)
			h.headers(req)
			if resp, err := h.client.Do(req); err == nil {
				resp.Body.Close()
			}
			cancel()
		}
		h.cancel()
		h.transport.CloseIdleConnections()
		h.wg.Wait()
		if h.c != nil {
			h.c.shutdown()
		}
	})
}

func (a *agent) startHTTP(ctx, procCtx context.Context, kill context.CancelFunc, req harness.Request, env []string) (*conn, *tailBuffer, func(), error) {
	found := make(chan string, 1)
	args := append(a.args(req), "--acp-transport", "streamable-http", "--host", "127.0.0.1", "--port", "0")
	binary, args := harness.CodeBuddyCommand(a.spec.Binary, args)
	cmd := harness.Command(procCtx, harness.Process{Binary: binary, Args: args, Workdir: req.Workdir, Env: env})
	// Keep stdin open: WorkBuddy exits when its parent closes stdin.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	cmd.Stdout = &endpointWriter{found: found}
	cmd.Stderr = &endpointWriter{found: found}
	if err = cmd.Start(); err != nil {
		stdin.Close()
		return nil, nil, nil, err
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	cleanupProcess := func() {
		_ = stdin.Close()
		select {
		case <-exited:
		case <-time.After(exitTimeout):
			if ctx.Err() != nil {
				kill()
			} else {
				_ = cmd.StopForeground()
			}
			<-exited
		}
	}
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	var endpoint string
	select {
	case endpoint = <-found:
	case <-exited:
		cleanupProcess()
		return nil, nil, nil, errors.New("acp: HTTP process exited before announcing its endpoint")
	case <-startCtx.Done():
		cleanupProcess()
		return nil, nil, nil, fmt.Errorf("acp: waiting for local HTTP endpoint: %w", startCtx.Err())
	}
	diag := &tailBuffer{max: 4096}
	h, err := newHTTPRPC(procCtx, startCtx, endpoint, diag)
	if err != nil {
		cleanupProcess()
		return nil, nil, nil, err
	}
	go func() {
		select {
		case <-exited:
			h.fail("ACP HTTP process exited")
		case <-h.ctx.Done():
		}
	}()
	return h.c, diag, func() { h.close(); cleanupProcess() }, nil
}
