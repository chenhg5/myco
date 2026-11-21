package remotedesktop

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrGone = errors.New("remote desktop is offline or the session expired")
var ErrBusy = errors.New("this computer already has a remote session")
var ErrDenied = errors.New("this remote session belongs to another controller")

// Broker only carries short-lived signaling. Pixels, sound and input never pass
// through it. Restarting the Hub drops registrations and pending negotiations.
type Broker struct {
	mu       sync.Mutex
	hosts    map[string]*host
	sessions map[string]*negotiation
	Now      func() time.Time
}
type host struct {
	device Device
	key    string
	owner  string
	at     time.Time
}
type negotiation struct {
	session        Session
	owner, hostKey string
	at             time.Time
}

func nonce() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func (b *Broker) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}
func (b *Broker) clean() {
	now := b.now()
	for id, h := range b.hosts {
		if now.Sub(h.at) > 30*time.Second {
			delete(b.hosts, id)
		}
	}
	for id, n := range b.sessions {
		if now.Sub(n.at) > 90*time.Second {
			delete(b.sessions, id)
		}
	}
}
func (b *Broker) Register(d Device) string { return b.RegisterOwned(d, "") }
func (b *Broker) RegisterOwned(d Device, owner string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clean()
	if b.hosts == nil {
		b.hosts = map[string]*host{}
		b.sessions = map[string]*negotiation{}
	}
	key := nonce()
	b.hosts[d.Machine] = &host{device: d, key: key, owner: owner, at: b.now()}
	for _, n := range b.sessions {
		if n.session.Machine == d.Machine {
			n.session.State = "closed"
			n.session.Offer = nil
			n.session.Answer = nil
		}
	}
	return key
}
func (b *Broker) Devices() []Device {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clean()
	out := []Device{}
	for _, h := range b.hosts {
		out = append(out, h.device)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Machine < out[j].Machine })
	return out
}
func cloneSession(s Session, offer bool) Session {
	out := s
	if s.Answer != nil {
		a := *s.Answer
		out.Answer = &a
	}
	out.Offer = nil
	if offer && s.Offer != nil {
		o := *s.Offer
		out.Offer = &o
	}
	return out
}
func (b *Broker) Requests(machine, key string) ([]Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clean()
	h := b.hosts[machine]
	if h == nil || h.key != key {
		return nil, ErrGone
	}
	h.at = b.now()
	out := []Session{}
	for _, n := range b.sessions {
		if n.session.Machine == machine && n.hostKey == key {
			out = append(out, cloneSession(n.session, true))
		}
	}
	return out, nil
}
func (b *Broker) Unregister(machine, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.hosts[machine]
	if h == nil || h.key != key {
		return ErrGone
	}
	delete(b.hosts, machine)
	for _, n := range b.sessions {
		if n.session.Machine == machine {
			n.session.State = "closed"
			n.session.Offer = nil
			n.session.Answer = nil
		}
	}
	return nil
}
func (b *Broker) Connect(owner, machine string, o Offer) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clean()
	h := b.hosts[machine]
	if h == nil {
		return Session{}, ErrGone
	}
	if o.Control && !h.device.Control {
		return Session{}, ErrDenied
	}
	for _, n := range b.sessions {
		if n.session.Machine == machine && n.session.State != "closed" && n.session.State != "failed" {
			return Session{}, ErrBusy
		}
	}
	s := Session{ID: nonce(), Machine: machine, State: "pending", Offer: &o}
	b.sessions[s.ID] = &negotiation{s, owner, h.key, b.now()}
	return cloneSession(s, false), nil
}
func (b *Broker) Session(owner, id string, close bool) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clean()
	n := b.sessions[id]
	if n == nil {
		return Session{}, ErrGone
	}
	if n.owner != owner {
		return Session{}, ErrDenied
	}
	n.at = b.now()
	if close {
		n.session.State = "closed"
		n.session.Offer = nil
		n.session.Answer = nil
	}
	return cloneSession(n.session, false), nil
}
func (b *Broker) Answer(machine, key, id string, answer *Description, message string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clean()
	n := b.sessions[id]
	h := b.hosts[machine]
	if n == nil || h == nil || h.key != key || n.hostKey != key || n.session.Machine != machine {
		return ErrGone
	}
	if n.session.State != "pending" && !(n.session.State == "connected" && answer == nil && message != "") {
		return ErrBusy
	}
	n.session.Offer = nil
	n.session.Answer = nil
	n.at = b.now()
	n.session.State = "connected"
	if answer == nil {
		n.session.State = "failed"
		n.session.Error = message
	} else {
		a := *answer
		n.session.Answer = &a
	}
	return nil
}

// Owner is used only for App-only hosts, identified by their existing console
// device credential. It never creates an agent or a persisted machine record.
func (b *Broker) Owner(id string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clean()
	h := b.hosts[id]
	if h == nil {
		return "", false
	}
	return h.owner, true
}
