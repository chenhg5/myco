package integration_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// Run traces: the recipient uploads the steps of a task's run; its sender,
// its recipient and the recipient's owner read them; nobody else does.
func TestTraces(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	enroll := func(name string) *client.Client {
		enr, err := admin.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: name, MaxUses: 1}, false)
		if err != nil {
			t.Fatal(err)
		}
		res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-" + name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		return h.client(res.Token)
	}
	boss, worker, nosy := enroll("boss"), enroll("worker"), enroll("nosy")
	task, err := boss.Send(c, api.SendRequest{To: "worker", Body: "run the tests"}, false)
	if err != nil {
		t.Fatal(err)
	}
	id := task.Message.ID
	if _, err := worker.Pull(c, 1, time.Second); err != nil {
		t.Fatal(err)
	}
	step := func(seq int, kind, data string) api.TraceEvent {
		return api.TraceEvent{Seq: seq, Kind: kind, Data: json.RawMessage(data)}
	}
	if err := nosy.PostTrace(c, id, []api.TraceEvent{step(1, "text", `{"kind":"text","text":"x"}`)}); err == nil {
		t.Fatal("someone other than the recipient uploaded a trace")
	}
	batch := []api.TraceEvent{step(1, "tool", `{"kind":"tool","tool":"Bash","input":"make test"}`), step(2, "text", `{"kind":"text","text":"green"}`)}
	if err := worker.PostTrace(c, id, batch); err != nil {
		t.Fatal(err)
	}
	// A retried upload is harmless.
	if err := worker.PostTrace(c, id, append(batch, step(3, "usage", `{"kind":"usage","duration_ms":5}`))); err != nil {
		t.Fatal(err)
	}
	for name, cl := range map[string]*client.Client{"sender": boss, "recipient": worker} {
		tr, err := cl.Trace(c, id, 0)
		if err != nil || len(tr.Events) != 3 || tr.Events[0].Kind != "tool" || tr.Status == "" {
			t.Fatalf("%s: %v %+v", name, err, tr)
		}
	}
	if tr, err := boss.Trace(c, id, 2); err != nil || len(tr.Events) != 1 || tr.Events[0].Seq != 3 {
		t.Fatalf("after 2: %v %+v", err, tr)
	}
	if _, err := nosy.Trace(c, id, 0); err == nil {
		t.Fatal("an unrelated agent read the trace")
	}

	// From the app: the person who sent a task reads its trace.
	sent, err := admin.ConsoleSend(c, api.SendRequest{To: "worker", Body: "and lint"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Pull(c, 1, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := worker.PostTrace(c, sent.Message.ID, []api.TraceEvent{step(1, "text", `{"kind":"text","text":"linting"}`)}); err != nil {
		t.Fatal(err)
	}
	if tr, err := admin.ConsoleTrace(c, sent.Message.ID, 0); err != nil || len(tr.Events) != 1 {
		t.Fatalf("console: %v %+v", err, tr)
	}
}
