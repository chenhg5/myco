package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// SetNotices enables /v1/console/notices: what each person should hear
// about. message looks up the message a notice is about (nil: none sent).
func (s *Server) SetNotices(n *app.NoticeService, message func(ctx context.Context, id string) (*domain.Message, error)) {
	s.notices, s.noticeMessage = n, message
}

func (s *Server) noticeRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/console/notices", s.admin(s.noticeFeed))
	m.HandleFunc("POST /v1/console/notices/read", s.admin(s.noticeRead))
	m.HandleFunc("GET /v1/console/notice-prefs", s.admin(s.noticePrefs))
	m.HandleFunc("PUT /v1/console/notice-prefs", s.admin(s.putNoticePrefs))
}

// noticeRecipient is whose notices a console request reads: the member
// behind it (the owner, for the hub's own admin token).
func (s *Server) noticeRecipient(r *http.Request) (string, error) {
	if s.notices == nil {
		return "", domain.Errf(domain.CodeNotFound, "this hub does not keep notices").
			WithSuggestion("upgrade the hub to v0.21.21 or later")
	}
	if superOf(r) != nil {
		return "", domain.Errf(domain.CodePermissionDenied, "a superagent does not read its owner's notices")
	}
	_, id := s.consoleIdentity(r)
	if id == "" {
		return "", domain.Errf(domain.CodePermissionDenied, "notices are a person's: this hub has no members yet")
	}
	return id, nil
}

func (s *Server) noticeFeed(w http.ResponseWriter, r *http.Request) {
	who, err := s.noticeRecipient(r)
	if err != nil {
		writeError(w, err)
		return
	}
	q := r.URL.Query()
	after, _ := strconv.ParseInt(q.Get("after"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	wait, _ := strconv.Atoi(q.Get("wait_seconds"))
	f, err := s.notices.Feed(r.Context(), who, after, limit, time.Duration(wait)*time.Second)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.NoticeFeed{Notices: make([]api.Notice, 0, len(f.Notices)), Last: f.Last, Badge: f.Badge}
	for _, n := range f.Notices {
		d := noticeDTO(n)
		if n.MessageID != "" && s.noticeMessage != nil {
			if m, err := s.noticeMessage(r.Context(), n.MessageID); err == nil {
				md := s.messageDTO(m)
				d.Message = &md
			}
		}
		out.Notices = append(out.Notices, d)
	}
	writeJSON(w, http.StatusOK, out)
}

func noticeDTO(n *domain.Notice) api.Notice {
	return api.Notice{
		Seq: n.Seq, ID: n.ID, Kind: string(n.Kind), Tier: string(n.Tier()), Key: n.Key, Ref: n.Ref, Target: n.Target,
		Project: n.Project, Actor: n.Actor, Subject: n.Subject, MessageID: n.MessageID, Announce: n.Announce, Quiet: n.Quiet,
		CreatedAt: n.CreatedAt, ReadAt: timePtr(n.ReadAt), ResolvedAt: timePtr(n.ResolvedAt),
	}
}

func (s *Server) noticeRead(w http.ResponseWriter, r *http.Request) {
	who, err := s.noticeRecipient(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req api.NoticeReadRequest
	if !decode(w, r, &req) {
		return
	}
	n, err := s.notices.Read(r.Context(), who, req.Target, req.Through)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.NoticeReadResponse{Changed: n})
}

func (s *Server) noticePrefs(w http.ResponseWriter, r *http.Request) {
	who, err := s.noticeRecipient(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.notices.Prefs(r.Context(), who)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, noticePrefsDTO(p))
}

func (s *Server) putNoticePrefs(w http.ResponseWriter, r *http.Request) {
	who, err := s.noticeRecipient(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req api.NoticePrefs
	if !decode(w, r, &req) {
		return
	}
	p, err := s.notices.SetPrefs(r.Context(), who, noticePrefsFromAPI(req))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, noticePrefsDTO(p))
}

func noticePrefsDTO(p *domain.NoticePrefs) api.NoticePrefs {
	out := api.NoticePrefs{Off: p.Off, PausedUntil: timePtr(p.PausedUntil), UpdatedAt: timePtr(p.UpdatedAt), Mutes: p.Mutes}
	if len(p.Tiers) > 0 {
		out.Tiers = map[string]bool{}
		for t, on := range p.Tiers {
			out.Tiers[string(t)] = on
		}
	}
	if q := p.Quiet; q != nil {
		out.Quiet = &api.NoticeQuiet{From: q.From, To: q.To, Zone: q.Zone}
	}
	return out
}

func noticePrefsFromAPI(in api.NoticePrefs) *domain.NoticePrefs {
	p := &domain.NoticePrefs{Off: in.Off, Mutes: in.Mutes}
	if in.PausedUntil != nil {
		p.PausedUntil = *in.PausedUntil
	}
	if len(in.Tiers) > 0 {
		p.Tiers = map[domain.NoticeTier]bool{}
		for t, on := range in.Tiers {
			p.Tiers[domain.NoticeTier(t)] = on
		}
	}
	if q := in.Quiet; q != nil {
		p.Quiet = &domain.QuietHours{From: q.From, To: q.To, Zone: q.Zone}
	}
	return p
}
