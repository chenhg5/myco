package httpapi

import (
	"testing"
	"time"
)

func TestASpawnGrantIsForItsNodeOnceAndNotForLong(t *testing.T) {
	g := newSpawnGrants()
	now := time.Unix(1_800_000_000, 0)
	id := g.add(&spawnGrant{node: "wsl-machine", provider: "prv_x"}, now)
	if _, ok := g.take(id, "other-machine", now); ok {
		t.Fatal("another node took the grant")
	}
	if _, ok := g.take(id, "wsl-machine", now); !ok {
		t.Fatal("its node could not take it")
	}
	if _, ok := g.take(id, "wsl-machine", now); ok {
		t.Fatal("taken twice")
	}
	late := g.add(&spawnGrant{node: "wsl-machine"}, now)
	if _, ok := g.take(late, "wsl-machine", now.Add(grantTTL)); ok {
		t.Fatal("taken after it expired")
	}
	if len(g.m) != 0 {
		t.Fatalf("expired grants are kept: %d", len(g.m))
	}
	gone := g.add(&spawnGrant{node: "wsl-machine"}, now)
	g.remove(gone) // the spawn request ended
	if _, ok := g.take(gone, "wsl-machine", now); ok {
		t.Fatal("taken after its request ended")
	}
}

func TestKeyReadsAreLimitedPerCaller(t *testing.T) {
	l := newRateLimit(3, time.Minute)
	now := time.Unix(1_800_000_000, 0)
	for i := 0; i < 3; i++ {
		if !l.allow("agent:a", now) {
			t.Fatalf("read %d refused", i)
		}
	}
	if l.allow("agent:a", now) {
		t.Fatal("a fourth read in a minute")
	}
	if !l.allow("agent:b", now) {
		t.Fatal("another caller was held to the first one's limit")
	}
	if !l.allow("agent:a", now.Add(time.Minute)) {
		t.Fatal("still refused a minute later")
	}
}
