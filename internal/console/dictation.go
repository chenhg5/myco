package console

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/chenhg5/myco/internal/client"
)

// Speaking instead of typing, with what the computer has, in the language
// the person chose for the app (not the system's):
//
//   - Listen (macOS): the app hears it itself (Apple's speech recognition)
//     and the page asks for the text as it grows, writing it into the box;
//   - Dictate (Windows): the system's voice typing, switched to that
//     language first, types into the box itself.
//
// POST /app/dictation {lang} starts either (listen answers with an id);
// GET /app/dictation/{id}?seq=n waits for text newer than seq; DELETE
// stops listening (the last words still come). One hearing at a time.

// Hearing is what one listening hears, as it grows.
type Hearing struct {
	mu      sync.Mutex
	seq     int
	text    string
	final   bool
	done    bool
	err     string
	changed chan struct{}
	stop    func()
	at      time.Time
}

func newHearing() *Hearing { return &Hearing{changed: make(chan struct{}), at: time.Now()} }

// Heard sets what has been heard so far (the whole text, not a piece).
func (h *Hearing) Heard(text string, final bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	h.text, h.final = text, final
	h.wake()
}

// End says listening is over; err is "" when it simply ended.
func (h *Hearing) End(err string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done {
		return
	}
	h.seq++
	h.done, h.err = true, err
	h.wake()
}

func (h *Hearing) wake() {
	close(h.changed)
	h.changed = make(chan struct{})
}

// HearingState is the answer to GET /app/dictation/{id}.
type HearingState struct {
	Seq   int    `json:"seq"`
	Text  string `json:"text"`
	Final bool   `json:"final,omitempty"`
	Done  bool   `json:"done,omitempty"`
	Error string `json:"error,omitempty"`
}

func (h *Hearing) state() HearingState {
	return HearingState{Seq: h.seq, Text: h.text, Final: h.final, Done: h.done, Error: h.err}
}

// wait returns the state once it is newer than seq, or after d.
func (h *Hearing) wait(ctx context.Context, seq int, d time.Duration) HearingState {
	t := time.NewTimer(d)
	defer t.Stop()
	for {
		h.mu.Lock()
		if h.seq > seq || h.done {
			st := h.state()
			h.mu.Unlock()
			return st
		}
		ch := h.changed
		h.mu.Unlock()
		select {
		case <-ch:
		case <-t.C:
			h.mu.Lock()
			st := h.state()
			h.mu.Unlock()
			return st
		case <-ctx.Done():
			return HearingState{Seq: seq}
		}
	}
}

type hearings struct {
	mu  sync.Mutex
	all map[string]*Hearing
}

func (b *Backend) dictationRoutes(m *http.ServeMux) {
	hs := &hearings{all: map[string]*Hearing{}}
	m.HandleFunc("POST /app/dictation", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Lang string `json:"lang"`
		}
		if r.ContentLength != 0 && !decode(w, r, &in) {
			return
		}
		lang := in.Lang
		if lang == "" {
			lang = "zh-CN"
		}
		switch {
		case b.Listen != nil:
			h := newHearing()
			hs.mu.Lock()
			for id, old := range hs.all {
				// One at a time: the one before stops; the finished ones go.
				old.mu.Lock()
				stop, done := old.stop, old.done
				old.mu.Unlock()
				if stop != nil && !done {
					stop()
				}
				if done || time.Since(old.at) > 10*time.Minute {
					delete(hs.all, id)
				}
			}
			id := hearingID()
			hs.all[id] = h
			hs.mu.Unlock()
			stop, err := b.Listen(lang, h)
			if err != nil {
				writeErr(w, &client.Error{Code: "unavailable", Message: err.Error(), Status: http.StatusConflict})
				return
			}
			h.mu.Lock()
			h.stop = stop
			h.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]string{"mode": "listen", "id": id})
		case b.Dictate != nil:
			if err := b.Dictate(lang); err != nil {
				writeErr(w, &client.Error{Code: "unavailable", Message: err.Error(), Status: http.StatusConflict})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"mode": "system"})
		default:
			writeErr(w, &client.Error{Code: "not_supported", Message: "dictation is only in the desktop app", Status: http.StatusNotFound})
		}
	})
	get := func(w http.ResponseWriter, r *http.Request) *Hearing {
		hs.mu.Lock()
		h := hs.all[r.PathValue("id")]
		hs.mu.Unlock()
		if h == nil {
			writeErr(w, &client.Error{Code: "not_found", Message: "no such listening", Status: http.StatusNotFound})
		}
		return h
	}
	m.HandleFunc("GET /app/dictation/{id}", func(w http.ResponseWriter, r *http.Request) {
		h := get(w, r)
		if h == nil {
			return
		}
		seq, _ := strconv.Atoi(r.URL.Query().Get("seq"))
		writeJSON(w, http.StatusOK, h.wait(r.Context(), seq, 8*time.Second))
	})
	m.HandleFunc("DELETE /app/dictation/{id}", func(w http.ResponseWriter, r *http.Request) {
		h := get(w, r)
		if h == nil {
			return
		}
		h.mu.Lock()
		stop := h.stop
		h.mu.Unlock()
		if stop != nil {
			stop()
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func hearingID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Utterances keeps what a recognizer heard when it starts over after a
// pause (Apple's does, on this Mac: after a pause its text is only the new
// sentence): what came before stays, the new sentence goes after it.
type Utterances struct{ done, last string }

// Add takes the recognizer's text now and returns all of it so far.
func (u *Utterances) Add(text string) string {
	if u.last != "" && startsOver(u.last, text) {
		u.done = joinText(u.done, u.last)
	}
	u.last = text
	return joinText(u.done, text)
}

// startsOver: next is a new sentence, not prev revised or carried on. A
// revision keeps the start (at least two letters, or all of a short
// beginning); a new sentence shares nothing with it.
func startsOver(prev, next string) bool {
	p, n := []rune(prev), []rune(next)
	same := 0
	for same < len(p) && same < len(n) && p[same] == n[same] {
		same++
	}
	if same >= 2 || same == len(p) || same == len(n) {
		return false
	}
	// The first word or two may still change as it hears more.
	return len(p) > 3
}

// joinText puts b after a: words in a Latin script get a space between.
func joinText(a, b string) string {
	if a == "" || b == "" {
		return a + b
	}
	last, _ := utf8.DecodeLastRuneInString(a)
	first, _ := utf8.DecodeRuneInString(b)
	if latin(last) && latin(first) {
		return a + " " + b
	}
	return a + b
}

func latin(r rune) bool {
	return r < 0x2E80 && (unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r))
}
