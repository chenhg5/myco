package remotedesktop

import (
	"errors"
	"testing"
	"time"
)

func TestSignalingLifecycle(t *testing.T) {
	now := time.Now()
	b := &Broker{Now: func() time.Time { return now }}
	key := b.Register(Device{Machine: "mac", Control: true})
	s, e := b.Connect("alice", "mac", Offer{PairingCode: "secret", Control: true})
	if e != nil || s.Offer != nil {
		t.Fatal(s, e)
	}
	if _, e = b.Session("bob", s.ID, false); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	rs, e := b.Requests("mac", key)
	if e != nil || len(rs) != 1 || rs[0].Offer.PairingCode != "secret" {
		t.Fatal(rs, e)
	}
	rs[0].Offer.PairingCode = "mutated"
	rs, _ = b.Requests("mac", key)
	if rs[0].Offer.PairingCode != "secret" {
		t.Fatal("mutable state escaped")
	}
	if _, e = b.Connect("alice", "mac", Offer{}); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	if e = b.Answer("mac", "wrong", s.ID, &Description{}, ""); !errors.Is(e, ErrGone) {
		t.Fatal(e)
	}
	if e = b.Answer("mac", key, s.ID, &Description{Type: "answer", SDP: "answer"}, ""); e != nil {
		t.Fatal(e)
	}
	got, e := b.Session("alice", s.ID, false)
	if e != nil || got.Answer.SDP != "answer" || got.State != "connected" {
		t.Fatal(got, e)
	}
	if _, e = b.Session("alice", s.ID, true); e != nil {
		t.Fatal(e)
	}
	rs, _ = b.Requests("mac", key)
	if rs[0].State != "closed" || rs[0].Offer != nil {
		t.Fatal(rs)
	}
	now = now.Add(31 * time.Second)
	if len(b.Devices()) != 0 {
		t.Fatal("stale host is online")
	}
	now = now.Add(time.Minute)
	if _, e = b.Session("alice", s.ID, false); !errors.Is(e, ErrGone) {
		t.Fatal(e)
	}
}
func TestHostReplacementRevokesAndViewOnly(t *testing.T) {
	b := &Broker{}
	key := b.Register(Device{Machine: "win"})
	if _, e := b.Connect("a", "win", Offer{Control: true}); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	s, _ := b.Connect("a", "win", Offer{})
	next := b.Register(Device{Machine: "win"})
	if next == key {
		t.Fatal("key reused")
	}
	if _, e := b.Requests("win", key); !errors.Is(e, ErrGone) {
		t.Fatal(e)
	}
	got, _ := b.Session("a", s.ID, false)
	if got.State != "closed" {
		t.Fatal(got)
	}
}

func TestHostReportsCaptureFailureAfterAnswer(t *testing.T) {
	b := &Broker{}
	key := b.Register(Device{Machine: "mac"})
	s, _ := b.Connect("me", "mac", Offer{})
	if e := b.Answer("mac", key, s.ID, &Description{Type: "answer", SDP: "secret sdp"}, ""); e != nil {
		t.Fatal(e)
	}
	if e := b.Answer("mac", key, s.ID, nil, "screen_capture_failed"); e != nil {
		t.Fatal(e)
	}
	out, e := b.Session("me", s.ID, false)
	if e != nil || out.State != "failed" || out.Answer != nil || out.Error != "screen_capture_failed" {
		t.Fatal(out, e)
	}
}
