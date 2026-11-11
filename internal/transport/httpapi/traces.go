package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// SetTraces enables run traces (the live view of a task's run).
func (s *Server) SetTraces(t *app.TraceService) { s.traces = t }

func (s *Server) tracesEnabled(w http.ResponseWriter) bool {
	if s.traces == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "this hub does not keep run traces"))
		return false
	}
	return true
}

func (s *Server) postTrace(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.tracesEnabled(w) {
		return
	}
	var req api.TraceUpload
	if !decode(w, r, &req) {
		return
	}
	in := make([]app.TraceInput, 0, len(req.Events))
	for _, e := range req.Events {
		in = append(in, app.TraceInput{Seq: e.Seq, Kind: e.Kind, Data: string(e.Data)})
	}
	if err := s.traces.Append(r.Context(), a, r.PathValue("id"), in); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"stored": len(in)})
}

func (s *Server) agentTrace(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	s.readTrace(w, r, app.TraceReader{Agent: a})
}

func (s *Server) consoleTrace(w http.ResponseWriter, r *http.Request) {
	name, memberID := s.consoleIdentity(r)
	s.readTrace(w, r, app.TraceReader{Console: app.ConsoleSender(name), MemberID: memberID})
}

func (s *Server) readTrace(w http.ResponseWriter, r *http.Request, reader app.TraceReader) {
	if !s.tracesEnabled(w) {
		return
	}
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	events, m, err := s.traces.Read(r.Context(), reader, r.PathValue("id"), after)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.Trace{Message: m.ID, Status: string(m.Status), Events: make([]api.TraceEvent, 0, len(events))}
	for _, e := range events {
		out.Events = append(out.Events, api.TraceEvent{Seq: e.Seq, Kind: e.Kind, Data: json.RawMessage(e.Data), At: e.At})
	}
	writeJSON(w, http.StatusOK, out)
}
