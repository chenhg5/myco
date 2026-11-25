package console

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/hubserver"
)

// The App's page renderer: a job for a page the agent may export goes to
// the window's renderer, whose file comes back to the agent; an app's
// error stays the app's.
func TestPageExportThroughTheWindowRenderer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h, err := hubserver.Build(hubserver.Options{DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: "admin-token-123456",
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, FilesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(h.Handler)
	defer func() { hs.Close(); h.Close() }()
	admin, _ := client.New(hs.URL, "admin-token-123456")
	ct, err := admin.CreateConsoleToken(ctx, "alice-mac")
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := client.New(hs.URL, ct.Token)
	reg, err := admin.Register(ctx, api.RegisterRequest{Name: "maker", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := client.New(hs.URL, reg.Token)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("index.html")
	_, _ = w.Write([]byte("<p>board</p>"))
	_ = z.Close()
	page, err := agent.PublishPage(ctx, client.PageInput{Name: "board"}, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	g, err := agent.RequestActionGrant(ctx, api.ActionGrantRequest{Target: api.ActionTarget{Kind: "page", ID: page.ID}, Permissions: []string{"page.export"}, Reason: "export"}, false)
	if err != nil {
		t.Fatal(err)
	}
	host, err := owner.RegisterActionHost(ctx, api.ActionHostRegister{Provider: "page", Generation: "app-1", Actions: []api.ActionDescriptor{pageExportAction}})
	if err != nil {
		t.Fatal(err)
	}

	b := &Backend{}
	mux := http.NewServeMux()
	b.pageRenderRoutes(mux)
	win := httptest.NewServer(mux)
	defer win.Close()
	// The window's renderer: takes the next export and answers it.
	answer := func(reply map[string]any) <-chan renderJob {
		got := make(chan renderJob, 1)
		go func() {
			res, err := http.Get(win.URL + "/app/page-render/next")
			if err != nil || res.StatusCode != 200 {
				return
			}
			var j renderJob
			_ = json.NewDecoder(res.Body).Decode(&j)
			res.Body.Close()
			got <- j
			body, _ := json.Marshal(reply)
			r2, err := http.Post(win.URL+"/app/page-render/"+j.ID+"/result", "application/json", bytes.NewReader(body))
			if err == nil {
				r2.Body.Close()
			}
		}()
		return got
	}
	run := func(key string) api.ActionHostJob {
		t.Helper()
		if _, err := agent.RunAction(ctx, api.ActionInvocation{GrantID: g.ID, Action: "page.export", Version: 1, IdempotencyKey: key, Args: json.RawMessage(`{"format":"svg"}`)}); err != nil {
			t.Fatal(err)
		}
		jobs, err := owner.ActionHostJobs(ctx, host.ID, 2*time.Second)
		if err != nil || len(jobs) != 1 {
			t.Fatalf("jobs: %+v %v", jobs, err)
		}
		return jobs[0]
	}

	hj := run("e1")
	got := answer(map[string]any{"media_type": "image/svg+xml", "data": base64.StdEncoding.EncodeToString([]byte("<svg/>")), "width": 800, "height": 600})
	done := b.runPageJob(ctx, owner, Hub{ID: "hub1"}, host, hj)
	j := <-got
	if j.HubID != "hub1" || j.PageID != page.ID || j.Action != "page.export" || string(j.Args) != `{"format":"svg"}` {
		t.Fatalf("render job: %+v", j)
	}
	if done.State != api.JobSucceeded || len(done.ArtifactIDs) != 1 {
		t.Fatalf("export: %+v", done)
	}
	var res struct {
		MediaType string `json:"media_type"`
		Width     int    `json:"width"`
	}
	if json.Unmarshal(done.Result, &res) != nil || res.MediaType != "image/svg+xml" || res.Width != 800 {
		t.Fatalf("result: %s", done.Result)
	}
	done.HostID, done.Generation = host.ID, host.Generation
	if _, err := owner.CompleteActionJob(ctx, hj.Job.ID, done); err != nil {
		t.Fatal(err)
	}
	if data, typ, err := agent.ActionArtifact(ctx, done.ArtifactIDs[0]); err != nil || string(data) != "<svg/>" || typ != "image/svg+xml" {
		t.Fatalf("file: %q %s %v", data, typ, err)
	}

	// While it renders, the window reads the page through the job; revoked,
	// the job stops at once: reads are refused and the window is told.
	hj = run("e3")
	type seen struct{ state, waitStop, after int }
	saw := make(chan seen, 1)
	go func() {
		res, err := http.Get(win.URL + "/app/page-render/next")
		if err != nil || res.StatusCode != 200 {
			return
		}
		var j renderJob
		_ = json.NewDecoder(res.Body).Decode(&j)
		res.Body.Close()
		var out seen
		if r, err := http.Get(win.URL + "/app/page-render/" + j.ID + "/state"); err == nil {
			out.state = r.StatusCode
			r.Body.Close()
		}
		if _, err := owner.RevokeActionGrant(ctx, g.ID); err != nil {
			return
		}
		if r, err := http.Get(win.URL + "/app/page-render/" + j.ID + "/wait"); err == nil {
			var v struct{ Stop bool }
			_ = json.NewDecoder(r.Body).Decode(&v)
			r.Body.Close()
			if v.Stop {
				out.waitStop = 1
			}
		}
		if r, err := http.Get(win.URL + "/app/page-render/" + j.ID + "/data?collection=items"); err == nil {
			out.after = r.StatusCode
			r.Body.Close()
		}
		saw <- out
	}()
	started := time.Now()
	done = b.runPageJob(ctx, owner, Hub{ID: "hub1"}, host, hj)
	if done.State != api.JobCancelled || time.Since(started) > 10*time.Second {
		t.Fatalf("revoked export: %+v after %v", done, time.Since(started))
	}
	if o := <-saw; o.state != 200 || o.waitStop != 1 || o.after != http.StatusGone {
		t.Fatalf("window during and after the revoke: %+v", o)
	}
	g, err = agent.RequestActionGrant(ctx, api.ActionGrantRequest{Target: api.ActionTarget{Kind: "page", ID: page.ID}, Permissions: []string{"page.export"}, Reason: "export again"}, false)
	if err != nil {
		t.Fatal(err)
	}

	// The app says no (a format it does not make): its code reaches the agent.
	hj = run("e2")
	_ = answer(map[string]any{"error": map[string]any{"code": "unsupported_format", "message": "no pptx here"}})
	done = b.runPageJob(ctx, owner, Hub{ID: "hub1"}, host, hj)
	if done.State != api.JobFailed || done.Error == nil || done.Error.Code != "unsupported_format" {
		t.Fatalf("app error: %+v", done)
	}
}
