package e2e

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func must(t *testing.T) *Identity {
	t.Helper()
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSealOpen(t *testing.T) {
	alice, bob, owner, eve := must(t), must(t), must(t), must(t)
	h := Header{From: "alice", To: "bob", Type: "task"}
	body, err := Seal(alice, h, []byte("deploy v2"), []string{bob.Public().Enc, alice.Public().Enc, owner.Public().Enc, bob.Public().Enc})
	if err != nil {
		t.Fatal(err)
	}
	if !IsSealed(body) || strings.Contains(body, "deploy") {
		t.Fatalf("not sealed: %s", body)
	}
	if n := len(Readers(body)); n != 3 {
		t.Fatalf("readers: %d (duplicates must be dropped)", n)
	}
	if Signer(body) != KeyID(alice.Public().Sign) {
		t.Fatal("signer id")
	}
	for name, id := range map[string]*Identity{"bob": bob, "alice": alice, "owner": owner} {
		pt, err := Open(id, h, body, alice.Public().Sign)
		if err != nil || string(pt) != "deploy v2" {
			t.Fatalf("%s: %v %q", name, err, pt)
		}
	}
	if _, err := Open(eve, h, body, alice.Public().Sign); !errors.Is(err, ErrNotForMe) {
		t.Fatalf("eve: %v", err)
	}
	// Rerouted or relabelled by the hub: the signature breaks.
	for _, bad := range []Header{{From: "alice", To: "carol", Type: "task"}, {From: "mallory", To: "bob", Type: "task"}, {From: "alice", To: "bob", Type: "note"}, {From: "alice", To: "bob", Type: "task", ReplyTo: "msg_x"}} {
		if _, err := Open(bob, bad, body, alice.Public().Sign); !errors.Is(err, ErrBadSignature) {
			t.Fatalf("header %+v: %v", bad, err)
		}
	}
	// Signed by someone else than the sender the reader knows.
	if _, err := Open(bob, h, body, eve.Public().Sign); !errors.Is(err, ErrUnknownSigner) {
		t.Fatalf("other signer: %v", err)
	}
	// A forged envelope signed by eve with alice's header.
	forged, _ := Seal(eve, h, []byte("rm -rf"), []string{bob.Public().Enc})
	if _, err := Open(bob, h, forged, alice.Public().Sign); !errors.Is(err, ErrUnknownSigner) {
		t.Fatalf("forged: %v", err)
	}
	// Tampered ciphertext.
	tampered := body[:len(body)-6] + "AAAAAA"
	if _, err := Open(bob, h, tampered, alice.Public().Sign); err == nil {
		t.Fatal("tampered body opened")
	}
	if _, err := Open(bob, h, "hello", alice.Public().Sign); !errors.Is(err, ErrMalformed) {
		t.Fatalf("plain text: %v", err)
	}
}

func TestIdentityRoundTrip(t *testing.T) {
	id := must(t)
	again, err := ParseIdentity(id.Marshal())
	if err != nil || again.Public() != id.Public() {
		t.Fatalf("round trip: %v", err)
	}
	if !id.Public().Valid() || (PublicKeys{Enc: "x", Sign: "y"}).Valid() {
		t.Fatal("Valid")
	}
	if _, err := ParseIdentity("garbage"); err == nil {
		t.Fatal("garbage parsed")
	}
	if f := Fingerprint(id.Public()); len(strings.Fields(f)) != 5 {
		t.Fatalf("fingerprint %q", f)
	}
}

func TestSealBytes(t *testing.T) {
	data := bytes.Repeat([]byte("file "), 1000)
	sealed, key, err := SealBytes(data)
	if err != nil || bytes.Contains(sealed, []byte("file file")) {
		t.Fatal("not sealed")
	}
	got, err := OpenBytes(sealed, key)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("open: %v", err)
	}
	_, other, _ := SealBytes(nil)
	if _, err := OpenBytes(sealed, other); err == nil {
		t.Fatal("wrong key opened")
	}
}

// An envelope sealed by an earlier build must keep opening: the format is
// stored in hub databases.
func TestVector(t *testing.T) {
	reader, err := ParseIdentity("A9B0xMRtqEYZTbLjwjjBEIUSp4S5blHx8PApOWwE_2A.NwenZwz40Et2j2IYVqCTG5IV3QYWn_dSY2-rGgMUW6o")
	if err != nil {
		t.Fatal(err)
	}
	const senderSign = "CQoL5OpmG3MKa57d60bTvneJpURh8B7PaY3flwlCDrk"
	const body = "myco-e2e:v1:eyJlcGsiOiJYa3Bxb0NqQUNpcUtKTl8xbURJbWZxQWphM0d1Y2d2S3hFUUxrTHlDcTFvIiwiayI6W3siaWQiOiI0Y2RlN2FlNzM5ZTZhZWQ4Iiwid2siOiI3Y0NoSmkyeFBFYkRGdUlEWTF2VzJVWFBqbUpINEcyWGJ3Mzd5bWhsejJhRnNsaDV4MnZkRGxHZTJuQkxlcUt0In1dLCJuIjoibWt0NVpnYzNTaTN5aEZneCIsImN0Ijoidi1PVjVKblh0UlpscXRCd1dTbHBiTF9uSmRMdy13IiwiZnJvbSI6IjViM2VhODI3Mzk0MDhmYzkiLCJzaWciOiJJejJiaDZPd3J2dGk4MV9oN254clVDZTFGZFFHRXRaMVZxMTdycHJxdVFWYXVMcHZlZUtQQmFpVUJHWGhZU3Q0YXIwWU0zS18xSmFwOUdlRUZaQU1EZyJ9"
	pt, err := Open(reader, Header{From: "a", To: "b", Type: "task"}, body, senderSign)
	if err != nil || string(pt) != "vector" {
		t.Fatalf("vector: %v %q", err, pt)
	}
}
