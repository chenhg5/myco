package app

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chenhg5/myco/internal/domain"
)

// Pins: what a conversation keeps at hand, above its messages in the App
// (置顶): a web address, a file, or one of its messages. A conversation is
// two parties (a person's console sender and an agent, or two agents);
// both see the same pins and either adds them: the person in the App, an
// agent with `myco pin add`. Kept in the hub's settings, one list per
// conversation, so every device agrees.

// MaxPins is how many pins a conversation keeps.
const MaxPins = 20

// Pin kinds.
const (
	PinKindLink    = "link"
	PinKindFile    = "file"
	PinKindMessage = "message"
)

// Pin is one thing a conversation keeps at hand.
type Pin struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// URL (link), the message's id (message).
	Target string `json:"target,omitempty"`
	// The file (file): what the App previews.
	File *PinFile `json:"file,omitempty"`
	// AddedBy is who pinned it (an agent's or a console sender's name).
	AddedBy string    `json:"added_by"`
	At      time.Time `json:"at"`
}

// PinFile is a pinned file, as attached to a message.
type PinFile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// PinService keeps each conversation's pins.
type PinService struct {
	settings domain.SettingRepository
	ids      IDGenerator
	clock    Clock
	mu       sync.Mutex
}

func NewPinService(settings domain.SettingRepository, ids IDGenerator, clock Clock) *PinService {
	return &PinService{settings: settings, ids: ids, clock: clock}
}

// pinPair is the conversation's two parties in one order, so that either
// side names the same list.
func pinPair(a, b string) (string, string, error) {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return "", "", domain.Errf(domain.CodeInvalidArgument, "a conversation has two parties: give both").
			WithSuggestion("pass the other party with --with")
	}
	if a == b {
		return "", "", domain.Errf(domain.CodeInvalidArgument, "a conversation is between two different parties")
	}
	if len(a) > 128 || len(b) > 128 {
		return "", "", domain.Errf(domain.CodeInvalidArgument, "name too long")
	}
	if b < a {
		a, b = b, a
	}
	return a, b, nil
}

func pinKey(a, b string) string { return "pins:" + a + "\x00" + b }

// Pins lists the conversation's pins, in their order.
func (s *PinService) Pins(ctx context.Context, a, b string) ([]Pin, error) {
	a, b, err := pinPair(a, b)
	if err != nil {
		return nil, err
	}
	return s.load(ctx, a, b)
}

func (s *PinService) load(ctx context.Context, a, b string) ([]Pin, error) {
	raw, err := s.settings.Get(ctx, pinKey(a, b))
	if err != nil || raw == "" {
		return []Pin{}, err
	}
	var ps []Pin
	if json.Unmarshal([]byte(raw), &ps) != nil || ps == nil {
		return []Pin{}, nil
	}
	return ps, nil
}

func (s *PinService) save(ctx context.Context, a, b string, ps []Pin) error {
	raw, _ := json.Marshal(ps)
	return s.settings.Set(ctx, pinKey(a, b), string(raw))
}

// AddPin is what to pin.
type AddPin struct {
	Kind   string
	Title  string
	Target string
	File   *PinFile
	By     string
}

func cleanTitle(t string) string {
	t = strings.Join(strings.Fields(t), " ")
	if utf8.RuneCountInString(t) > 80 {
		r := []rune(t)
		t = string(r[:79]) + "…"
	}
	return t
}

// Add pins something to the conversation (last); the same thing pinned
// again is not added twice, its title is updated.
func (s *PinService) Add(ctx context.Context, a, b string, in AddPin) (Pin, error) {
	a, b, err := pinPair(a, b)
	if err != nil {
		return Pin{}, err
	}
	p := Pin{Kind: in.Kind, Title: cleanTitle(in.Title), AddedBy: in.By}
	switch in.Kind {
	case PinKindLink:
		u, err := url.Parse(strings.TrimSpace(in.Target))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(in.Target) > 2048 {
			return Pin{}, domain.Errf(domain.CodeInvalidArgument, "a link is an http or https address (2048 characters at most)")
		}
		p.Target = u.String()
		if p.Title == "" {
			p.Title = cleanTitle(u.Host + strings.TrimSuffix(u.Path, "/"))
		}
	case PinKindMessage:
		id := strings.TrimSpace(in.Target)
		if id == "" || len(id) > 128 {
			return Pin{}, domain.Errf(domain.CodeInvalidArgument, "which message: give its id")
		}
		p.Target = id
		if p.Title == "" {
			p.Title = id
		}
	case PinKindFile:
		if in.File == nil || strings.TrimSpace(in.File.ID) == "" || len(in.File.ID) > 128 {
			return Pin{}, domain.Errf(domain.CodeInvalidArgument, "which file: give its id")
		}
		f := *in.File
		f.Name = cleanTitle(f.Name)
		p.File = &f
		if p.Title == "" {
			p.Title = f.Name
		}
		if p.Title == "" {
			p.Title = f.ID
		}
	default:
		return Pin{}, domain.Errf(domain.CodeInvalidArgument, "a pin is a link, a file or a message")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load(ctx, a, b)
	if err != nil {
		return Pin{}, err
	}
	for i := range ps {
		if ps[i].Kind == p.Kind && ps[i].Target == p.Target && (p.File == nil || ps[i].File != nil && ps[i].File.ID == p.File.ID) {
			if in.Title != "" {
				ps[i].Title = p.Title
				if err := s.save(ctx, a, b, ps); err != nil {
					return Pin{}, err
				}
			}
			return ps[i], nil
		}
	}
	if len(ps) >= MaxPins {
		return Pin{}, domain.Errf(domain.CodeConflict, "a conversation keeps %d pins at most", MaxPins).
			WithSuggestion("unpin one first: myco pin rm")
	}
	p.ID = s.ids.New("pin")
	p.At = s.clock.Now().UTC()
	ps = append(ps, p)
	if err := s.save(ctx, a, b, ps); err != nil {
		return Pin{}, err
	}
	return p, nil
}

// Rename gives a pin another title.
func (s *PinService) Rename(ctx context.Context, a, b, id, title string) (Pin, error) {
	a, b, err := pinPair(a, b)
	if err != nil {
		return Pin{}, err
	}
	title = cleanTitle(title)
	if title == "" {
		return Pin{}, domain.Errf(domain.CodeInvalidArgument, "a pin needs a title")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load(ctx, a, b)
	if err != nil {
		return Pin{}, err
	}
	for i := range ps {
		if ps[i].ID == id {
			ps[i].Title = title
			return ps[i], s.save(ctx, a, b, ps)
		}
	}
	return Pin{}, domain.Errf(domain.CodeNotFound, "no pin %s in this conversation", id)
}

// Remove unpins one.
func (s *PinService) Remove(ctx context.Context, a, b, id string) error {
	a, b, err := pinPair(a, b)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load(ctx, a, b)
	if err != nil {
		return err
	}
	out := ps[:0]
	found := false
	for _, p := range ps {
		if p.ID == id {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return domain.Errf(domain.CodeNotFound, "no pin %s in this conversation", id)
	}
	return s.save(ctx, a, b, out)
}

// Order puts the pins in this order (ids not named keep theirs, after).
func (s *PinService) Order(ctx context.Context, a, b string, ids []string) ([]Pin, error) {
	a, b, err := pinPair(a, b)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load(ctx, a, b)
	if err != nil {
		return nil, err
	}
	byID := map[string]Pin{}
	for _, p := range ps {
		byID[p.ID] = p
	}
	out := make([]Pin, 0, len(ps))
	used := map[string]bool{}
	for _, id := range ids {
		if p, ok := byID[id]; ok && !used[id] {
			used[id] = true
			out = append(out, p)
		}
	}
	for _, p := range ps {
		if !used[p.ID] {
			out = append(out, p)
		}
	}
	return out, s.save(ctx, a, b, out)
}
