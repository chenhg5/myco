package main

import (
	"net/http"
	"testing"
)

func TestTrayStatus(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/overview" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"hubs":[
			{"name":"home","ok":true,"overview":{"agents":[
				{"agent":{"binding":{"harness":"claude"}},"state":"working"},
				{"agent":{"binding":{"harness":"codex"}},"state":"working"},
				{"agent":{"binding":{"harness":"exec"}},"state":"idle"},
				{"agent":{"binding":{"harness":"claude"}},"state":"offline"},
				{"agent":{"binding":{"harness":"machine"}},"state":"offline"}]}},
			{"name":"VPS-hub","ok":false,"error":"timeout"}]}`))
	})
	st := readTrayStatus(h)
	if st.Working != 2 || st.Idle != 1 || st.Offline != 1 || st.Pending != 0 || st.Hubs != 2 || len(st.Down) != 1 || st.Down[0] != "VPS-hub" {
		t.Fatalf("status: %+v (machine nodes must not count)", st)
	}
	if got := trayText(true).summary(st); got != "2 个工作中 · 1 个空闲 · 1 个离线" {
		t.Errorf("zh summary: %q", got)
	}
	if got := trayText(false).summary(st); got != "2 working · 1 idle · 1 offline" {
		t.Errorf("en summary: %q", got)
	}
	if got := trayText(true).summary(trayStatus{}); got != "还没有连接 Hub" {
		t.Errorf("no hubs: %q", got)
	}
}
