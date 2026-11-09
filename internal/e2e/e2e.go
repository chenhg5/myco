// Package e2e is myco's end-to-end encryption of message bodies (see
// docs/design/e2e-encryption.md).
//
// Every identity (an agent, an app device) has an X25519 key to receive and
// an Ed25519 key to sign. A message is sealed with a random content key
// (AES-256-GCM); the content key is wrapped for each reader with a key
// derived (HKDF-SHA256) from X25519 between a one-message ephemeral key and
// the reader's key. The sender signs the routing header and the ciphertext,
// so the hub can neither read nor forge nor reroute a message. Only the
// standard library is used.
package e2e

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Prefix starts every sealed body.
const Prefix = "myco-e2e:v1:"

var (
	// ErrNotForMe: the message was not sealed for this identity.
	ErrNotForMe = errors.New("e2e: this message is not encrypted for this identity")
	// ErrBadSignature: the signature does not match (tampered, or rerouted).
	ErrBadSignature = errors.New("e2e: bad signature")
	// ErrUnknownSigner: the signing key is not one the reader knows for the sender.
	ErrUnknownSigner = errors.New("e2e: unknown signing key")
	// ErrMalformed: not a well-formed envelope.
	ErrMalformed = errors.New("e2e: malformed envelope")
)

var b64 = base64.RawURLEncoding

// Identity is one party's key pair.
type Identity struct {
	enc  *ecdh.PrivateKey
	sign ed25519.PrivateKey
}

// PublicKeys are an identity's public halves, as the hub stores them.
type PublicKeys struct {
	Enc  string `json:"enc_key"`  // X25519, base64url
	Sign string `json:"sign_key"` // Ed25519, base64url
}

// NewIdentity makes a fresh key pair.
func NewIdentity() (*Identity, error) {
	enc, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	_, sign, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{enc: enc, sign: sign}, nil
}

// Marshal encodes the private keys ("<x25519>.<ed25519 seed>", base64url) for
// a 0600 file or a keychain.
func (id *Identity) Marshal() string {
	return b64.EncodeToString(id.enc.Bytes()) + "." + b64.EncodeToString(id.sign.Seed())
}

// ParseIdentity decodes Marshal's output.
func ParseIdentity(s string) (*Identity, error) {
	a, b, ok := strings.Cut(strings.TrimSpace(s), ".")
	if !ok {
		return nil, errors.New("e2e: bad private key")
	}
	ek, err := b64.DecodeString(a)
	if err != nil {
		return nil, errors.New("e2e: bad private key")
	}
	seed, err := b64.DecodeString(b)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("e2e: bad private key")
	}
	enc, err := ecdh.X25519().NewPrivateKey(ek)
	if err != nil {
		return nil, errors.New("e2e: bad private key")
	}
	return &Identity{enc: enc, sign: ed25519.NewKeyFromSeed(seed)}, nil
}

// Public returns the identity's public keys.
func (id *Identity) Public() PublicKeys {
	return PublicKeys{Enc: b64.EncodeToString(id.enc.PublicKey().Bytes()),
		Sign: b64.EncodeToString(id.sign.Public().(ed25519.PublicKey))}
}

// KeyID is a short id of a public key (encryption or signing).
func KeyID(pub string) string {
	raw, _ := b64.DecodeString(pub)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:8])
}

// Fingerprint is a public key pair for people to compare (groups of four).
func Fingerprint(k PublicKeys) string {
	h := sha256.Sum256([]byte(k.Enc + "|" + k.Sign))
	s := strings.ToUpper(hex.EncodeToString(h[:10]))
	var parts []string
	for i := 0; i < len(s); i += 4 {
		parts = append(parts, s[i:i+4])
	}
	return strings.Join(parts, " ")
}

// Valid reports whether k holds two well-formed public keys.
func (k PublicKeys) Valid() bool {
	e, err1 := b64.DecodeString(k.Enc)
	s, err2 := b64.DecodeString(k.Sign)
	if err1 != nil || err2 != nil || len(s) != ed25519.PublicKeySize {
		return false
	}
	_, err := ecdh.X25519().NewPublicKey(e)
	return err == nil
}

// Header is what the sender signs besides the ciphertext: where the message
// goes and what it is. A hub that reroutes or relabels a message breaks the
// signature.
type Header struct {
	From, To, Type, ReplyTo string
}

type wrapped struct {
	ID string `json:"id"`
	WK string `json:"wk"`
}

type envelope struct {
	EPK  string    `json:"epk"`
	K    []wrapped `json:"k"`
	N    string    `json:"n"`
	CT   string    `json:"ct"`
	From string    `json:"from"` // key id of the signing key
	Sig  string    `json:"sig"`
}

// IsSealed reports whether body is an envelope.
func IsSealed(body string) bool { return strings.HasPrefix(body, Prefix) }

// Seal encrypts payload for readers (their X25519 public keys, base64url;
// duplicates are dropped) and signs it as id.
func Seal(id *Identity, h Header, payload []byte, readers []string) (string, error) {
	if len(readers) == 0 {
		return "", errors.New("e2e: no readers")
	}
	ck := make([]byte, 32)
	if _, err := rand.Read(ck); err != nil {
		return "", err
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	env := envelope{EPK: b64.EncodeToString(eph.PublicKey().Bytes()), From: KeyID(id.Public().Sign)}
	seen := map[string]bool{}
	for _, r := range readers {
		kid := KeyID(r)
		if seen[kid] {
			continue
		}
		seen[kid] = true
		raw, err := b64.DecodeString(r)
		if err != nil {
			return "", fmt.Errorf("e2e: bad reader key: %w", err)
		}
		pub, err := ecdh.X25519().NewPublicKey(raw)
		if err != nil {
			return "", fmt.Errorf("e2e: bad reader key: %w", err)
		}
		wk, err := wrapKey(eph, pub)
		if err != nil {
			return "", err
		}
		sealed, err := gcmSeal(wk, make([]byte, 12), ck, nil)
		if err != nil {
			return "", err
		}
		env.K = append(env.K, wrapped{ID: kid, WK: b64.EncodeToString(sealed)})
	}
	sort.Slice(env.K, func(i, j int) bool { return env.K[i].ID < env.K[j].ID })
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	env.N = b64.EncodeToString(nonce)
	ct, err := gcmSeal(ck, nonce, payload, []byte(Prefix))
	if err != nil {
		return "", err
	}
	env.CT = b64.EncodeToString(ct)
	env.Sig = b64.EncodeToString(ed25519.Sign(id.sign, signed(h, env)))
	j, _ := json.Marshal(env)
	return Prefix + b64.EncodeToString(j), nil
}

// Signer returns the key id of the key that signed body ("" if malformed),
// so the reader can look up which public key to verify with.
func Signer(body string) string {
	env, err := parse(body)
	if err != nil {
		return ""
	}
	return env.From
}

// Open verifies body's signature with signKey (the sender's Ed25519 public
// key, base64url, as the reader knows it) and decrypts it as id.
func Open(id *Identity, h Header, body, signKey string) ([]byte, error) {
	env, err := parse(body)
	if err != nil {
		return nil, err
	}
	if KeyID(signKey) != env.From {
		return nil, ErrUnknownSigner
	}
	pub, err := b64.DecodeString(signKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, ErrUnknownSigner
	}
	sig, err := b64.DecodeString(env.Sig)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), signed(h, *env), sig) {
		return nil, ErrBadSignature
	}
	mine := KeyID(id.Public().Enc)
	var wk string
	for _, k := range env.K {
		if k.ID == mine {
			wk = k.WK
			break
		}
	}
	if wk == "" {
		return nil, ErrNotForMe
	}
	epkRaw, err := b64.DecodeString(env.EPK)
	if err != nil {
		return nil, ErrMalformed
	}
	epk, err := ecdh.X25519().NewPublicKey(epkRaw)
	if err != nil {
		return nil, ErrMalformed
	}
	key, err := unwrapKey(id.enc, epk)
	if err != nil {
		return nil, err
	}
	sealed, err := b64.DecodeString(wk)
	if err != nil {
		return nil, ErrMalformed
	}
	ck, err := gcmOpen(key, make([]byte, 12), sealed, nil)
	if err != nil {
		return nil, ErrMalformed
	}
	nonce, err1 := b64.DecodeString(env.N)
	ct, err2 := b64.DecodeString(env.CT)
	if err1 != nil || err2 != nil {
		return nil, ErrMalformed
	}
	pt, err := gcmOpen(ck, nonce, ct, []byte(Prefix))
	if err != nil {
		return nil, ErrMalformed
	}
	return pt, nil
}

// Readers lists the key ids body is sealed for.
func Readers(body string) []string {
	env, err := parse(body)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(env.K))
	for _, k := range env.K {
		out = append(out, k.ID)
	}
	return out
}

func parse(body string) (*envelope, error) {
	if !IsSealed(body) {
		return nil, ErrMalformed
	}
	j, err := b64.DecodeString(strings.TrimPrefix(body, Prefix))
	if err != nil {
		return nil, ErrMalformed
	}
	var env envelope
	if json.Unmarshal(j, &env) != nil || env.EPK == "" || env.CT == "" || env.Sig == "" || len(env.K) == 0 {
		return nil, ErrMalformed
	}
	return &env, nil
}

// signed is the byte string the sender signs.
func signed(h Header, env envelope) []byte {
	ct := sha256.Sum256([]byte(env.CT))
	var b bytes.Buffer
	b.WriteString("myco-e2e v1\n")
	for _, s := range []string{h.From, h.To, h.Type, h.ReplyTo, env.EPK, env.N, hex.EncodeToString(ct[:]), env.From} {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	for _, k := range env.K {
		b.WriteString(k.ID + ":" + k.WK + "\n")
	}
	return b.Bytes()
}

func wrapKey(eph *ecdh.PrivateKey, reader *ecdh.PublicKey) ([]byte, error) {
	shared, err := eph.ECDH(reader)
	if err != nil {
		return nil, err
	}
	salt := append(append([]byte{}, eph.PublicKey().Bytes()...), reader.Bytes()...)
	return hkdf.Key(sha256.New, shared, salt, "myco-e2e v1 wrap", 32)
}

func unwrapKey(me *ecdh.PrivateKey, epk *ecdh.PublicKey) ([]byte, error) {
	shared, err := me.ECDH(epk)
	if err != nil {
		return nil, err
	}
	salt := append(append([]byte{}, epk.Bytes()...), me.PublicKey().Bytes()...)
	return hkdf.Key(sha256.New, shared, salt, "myco-e2e v1 wrap", 32)
}

func gcmSeal(key, nonce, pt, ad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return g.Seal(nil, nonce, pt, ad), nil
}

func gcmOpen(key, nonce, ct, ad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return g.Open(nil, nonce, ct, ad)
}

// SealBytes encrypts file content with a fresh key (returned, base64url):
// attachments are sealed this way and their key travels inside the message.
func SealBytes(data []byte) (sealed []byte, key string, err error) {
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, "", err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, "", err
	}
	ct, err := gcmSeal(k, nonce, data, []byte("myco-e2e v1 file"))
	if err != nil {
		return nil, "", err
	}
	return append(nonce, ct...), b64.EncodeToString(k), nil
}

// OpenBytes decrypts SealBytes' output.
func OpenBytes(sealed []byte, key string) ([]byte, error) {
	k, err := b64.DecodeString(key)
	if err != nil || len(k) != 32 || len(sealed) < 12 {
		return nil, ErrMalformed
	}
	pt, err := gcmOpen(k, sealed[:12], sealed[12:], []byte("myco-e2e v1 file"))
	if err != nil {
		return nil, ErrMalformed
	}
	return pt, nil
}
