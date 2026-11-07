package security

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// base32 (RFC 4648) alphabet, lower-cased, no padding.
var (
	idRe    = regexp.MustCompile(`^msg_[a-z2-7]{16}$`)
	tokenRe = regexp.MustCompile(`^myco_[a-z2-7]{39}$`)
	hashRe  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func TestIDsFormat(t *testing.T) {
	for _, prefix := range []string{"msg", "agt", "f"} {
		id := IDs{}.New(prefix)
		if !strings.HasPrefix(id, prefix+"_") {
			t.Fatalf("New(%q) = %q, want prefix %q", prefix, id, prefix+"_")
		}
		// 10 random bytes = 80 bits -> 16 base32 chars.
		rest := strings.TrimPrefix(id, prefix+"_")
		if len(rest) != 16 || strings.Trim(rest, "abcdefghijklmnopqrstuvwxyz234567") != "" {
			t.Fatalf("New(%q) random part %q: want 16 lower-case base32 chars", prefix, rest)
		}
	}
	if id := (IDs{}).New("msg"); !idRe.MatchString(id) {
		t.Fatalf("id %q does not match %s", id, idRe)
	}
}

func TestIDsUniqueConcurrent(t *testing.T) {
	const workers, per = 8, 2000
	var mu sync.Mutex
	seen := make(map[string]struct{}, workers*per)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]string, 0, per)
			for range per {
				local = append(local, IDs{}.New("msg"))
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate id %q", id)
				}
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()
}

func TestTokensIssueFormatAndEntropy(t *testing.T) {
	var tok Tokens
	seen := map[string]bool{}
	// Count characters per position: a constant position would mean the
	// randomness is not reaching the encoded token.
	const n = 500
	pos := make([]map[byte]bool, 39)
	for i := range pos {
		pos[i] = map[byte]bool{}
	}
	for range n {
		plain, hash, err := tok.Issue()
		if err != nil {
			t.Fatal(err)
		}
		// 24 random bytes = 192 bits -> 39 base32 chars after "myco_".
		if !tokenRe.MatchString(plain) {
			t.Fatalf("token %q does not match %s", plain, tokenRe)
		}
		if !hashRe.MatchString(hash) {
			t.Fatalf("hash %q is not 64 hex chars", hash)
		}
		if hash == plain || strings.Contains(hash, strings.TrimPrefix(plain, "myco_")) {
			t.Fatal("hash must not reveal the token")
		}
		if seen[plain] {
			t.Fatalf("duplicate token %q", plain)
		}
		seen[plain] = true
		for i, c := range []byte(strings.TrimPrefix(plain, "myco_")) {
			pos[i][c] = true
		}
	}
	// The last char carries only 192 mod 5 = 2 random bits (4 values).
	for i, set := range pos[:38] {
		if len(set) < 16 {
			t.Errorf("position %d only saw %d distinct chars over %d tokens", i, len(set), n)
		}
	}
}

func TestTokensHashVerify(t *testing.T) {
	var tok Tokens
	plain, hash, err := tok.Issue()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(plain))
	if want := hex.EncodeToString(sum[:]); hash != want {
		t.Fatalf("hash = %s, want sha256 hex %s", hash, want)
	}
	if tok.Hash(plain) != hash {
		t.Fatal("Hash is not deterministic: issued token does not verify")
	}
	other, _, _ := tok.Issue()
	for _, wrong := range []string{"", other, plain + "x", strings.ToUpper(plain), plain[:len(plain)-1], " " + plain} {
		if tok.Hash(wrong) == hash {
			t.Errorf("wrong token %q verifies", wrong)
		}
	}
	// Known vector pins the algorithm (stored hashes must stay valid).
	if got := tok.Hash("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("Hash(abc) = %s", got)
	}
}
