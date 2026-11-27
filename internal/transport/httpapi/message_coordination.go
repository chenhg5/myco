package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func (s *Server) messageCoordinationRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/messages/{id}/tree", s.adminOrAgent(s.messageTree))
	m.HandleFunc("POST /v1/messages/{id}/reassign", s.adminOrAgent(s.reassignMessage))
}
func (s *Server) messageActor(r *http.Request) app.Actor {
	if a, _ := r.Context().Value(callerAgentKey{}).(*domain.Agent); a != nil {
		return app.Actor{Agent: a}
	}
	return s.actorOf(r)
}
func treeNode(n *app.TaskTree, at time.Time) api.TaskTreeNode {
	m := n.Message
	age := int64(at.Sub(m.CreatedAt).Seconds())
	if age < 0 {
		age = 0
	}
	out := api.TaskTreeNode{ID: m.ID, From: string(m.From), Agent: string(m.To), Status: string(m.Status), AgeSeconds: age, ThreadID: m.ThreadID, Project: m.Project, ParentID: m.ParentID, Relation: n.Relation, Reference: n.Reference, WaitingOn: []api.TaskTreeWait{}, Children: []api.TaskTreeNode{}}
	for _, w := range n.Waits {
		out.WaitingOn = append(out.WaitingOn, api.TaskTreeWait{Kind: w.Kind, ID: w.ID})
	}
	for _, c := range n.Children {
		out.Children = append(out.Children, treeNode(c, at))
	}
	return out
}
func (s *Server) messageTree(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if q := r.URL.Query().Get("limit"); q != "" {
		var err error
		limit, err = strconv.Atoi(q)
		if err != nil {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "invalid limit"))
			return
		}
	}
	out, err := s.messages.MessageTree(r.Context(), s.messageActor(r), r.PathValue("id"), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.TaskTreeResponse{Root: treeNode(out.Root, out.At), AsOf: out.At, Truncated: out.Truncated})
}
func (s *Server) reassignMessage(w http.ResponseWriter, r *http.Request) {
	var in api.ReassignRequest
	if !decode(w, r, &in) {
		return
	}
	out, err := s.messages.Reassign(r.Context(), s.messageActor(r), r.PathValue("id"), in.To, in.Note, dryRun(r))
	if err != nil {
		writeError(w, err)
		return
	}
	m := out.Message
	writeJSON(w, http.StatusOK, api.ReassignResponse{ID: m.ID, From: string(m.From), To: string(m.To), PreviousTo: string(out.PreviousTo), ThreadID: m.ThreadID, Project: m.Project, Status: string(m.Status), DryRun: out.DryRun})
}
