package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/e2e"
)

// BroadcastTo is the recipient a broadcast is sealed for (one envelope for
// every copy).
const BroadcastTo = "@broadcast"

// Sealer makes a client encrypt what it sends and decrypt what it receives
// when the hub has end-to-end encryption on (docs/design/e2e-encryption.md).
type Sealer struct {
	// Me is who the client speaks as: the agent's name, or
	// "@console:<member>" for a member's device.
	Me string
	// ID is this identity's key pair.
	ID *e2e.Identity
	// Device: the keys belong to a console device (registered with
	// PUT /v1/console/me/keys) rather than an agent.
	Device bool
	// Known pins agents' keys on first sight (nil = no pinning).
	Known *e2e.Known

	mu         sync.Mutex
	on         bool
	onAt       time.Time
	registered bool
	keys       map[string]cachedKeys
	msgs       map[string]msgRef
	files      map[string]fileKey
}

type cachedKeys struct {
	at   time.Time
	list []api.KeyHolder
}

type msgRef struct{ from, to string }

type fileKey struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type,omitempty"`
	FileID      string `json:"file_id"`
}

// payload is what an envelope holds.
type payload struct {
	Body  string    `json:"body"`
	Files []fileKey `json:"files,omitempty"`
}

// E2EEnabled reports whether the hub has end-to-end encryption on (cached
// for 30 seconds; false for hubs that do not support it).
func (c *Client) E2EEnabled(ctx context.Context) bool {
	s := c.Sealer
	if s == nil {
		return false
	}
	s.mu.Lock()
	if time.Since(s.onAt) < 30*time.Second {
		on := s.on
		s.mu.Unlock()
		return on
	}
	s.mu.Unlock()
	var out api.HubSettings
	err := c.do(ctx, http.MethodGet, "/v1/hub/settings", nil, nil, &out)
	on := err == nil && out.E2E
	s.mu.Lock()
	s.on, s.onAt = on, time.Now()
	s.mu.Unlock()
	if on {
		_ = c.registerKeys(ctx)
	}
	return on
}

// HubSettings reads the hub's settings.
func (c *Client) HubSettings(ctx context.Context) (*api.HubSettings, error) {
	var out api.HubSettings
	return &out, c.do(ctx, http.MethodGet, "/v1/hub/settings", nil, nil, &out)
}

// SetHubSettings changes them (the hub's owner).
func (c *Client) SetHubSettings(ctx context.Context, in api.HubSettings) (*api.HubSettings, error) {
	var out api.HubSettings
	return &out, c.do(ctx, http.MethodPut, "/v1/hub/settings", nil, in, &out)
}

// KeysOf returns identities' public keys as the hub has them.
func (c *Client) KeysOf(ctx context.Context, who ...string) (*api.KeyList, error) {
	var out api.KeyList
	return &out, c.do(ctx, http.MethodGet, "/v1/keys", url.Values{"who": {strings.Join(who, ",")}}, nil, &out)
}

// ReadersOf returns who may read a message from one identity to another.
func (c *Client) ReadersOf(ctx context.Context, from, to string) (*api.Readers, error) {
	var out api.Readers
	return &out, c.do(ctx, http.MethodGet, "/v1/keys/readers", url.Values{"from": {from}, "to": {to}}, nil, &out)
}

// RegisterKeys uploads this identity's public keys (once per client).
func (c *Client) RegisterKeys(ctx context.Context) error { return c.registerKeys(ctx) }

func (c *Client) registerKeys(ctx context.Context) error {
	s := c.Sealer
	if s == nil || s.ID == nil {
		return nil
	}
	s.mu.Lock()
	done := s.registered
	s.mu.Unlock()
	if done {
		return nil
	}
	pub := s.ID.Public()
	path := "/v1/me/keys"
	if s.Device {
		path = "/v1/console/me/keys"
	}
	if err := c.do(ctx, http.MethodPut, path, nil, api.PutKeysRequest{EncKey: pub.Enc, SignKey: pub.Sign}, nil); err != nil {
		return err
	}
	s.mu.Lock()
	s.registered = true
	s.mu.Unlock()
	return nil
}

func (s *Sealer) remember(m *api.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.msgs == nil {
		s.msgs = map[string]msgRef{}
	}
	if len(s.msgs) > 5000 {
		s.msgs = map[string]msgRef{}
	}
	s.msgs[m.ID] = msgRef{from: m.From, to: m.To}
}

// partiesOf returns a message's sender and recipient (from what this client
// has seen, else from the hub).
func (c *Client) partiesOf(ctx context.Context, id string) (msgRef, error) {
	s := c.Sealer
	s.mu.Lock()
	r, ok := s.msgs[id]
	s.mu.Unlock()
	if ok {
		return r, nil
	}
	var m api.Message
	if err := c.do(ctx, http.MethodGet, "/v1/messages/"+esc(id), nil, nil, &m); err != nil {
		return msgRef{}, err
	}
	s.remember(&m)
	return msgRef{from: m.From, to: m.To}, nil
}

// pin checks agents' keys against the ones seen before. Members' devices
// are not pinned (people add devices); see the design's known limits.
func (s *Sealer) pin(h api.KeyHolder) error {
	if s.Known == nil || strings.HasPrefix(h.Who, "@") || h.Who == s.Me {
		return nil
	}
	return s.Known.Check(h.Who, e2e.PublicKeys{Enc: h.EncKey, Sign: h.SignKey})
}

// seal encrypts body for a message of type typ from s.Me to to, when the
// hub has encryption on and every side has keys; otherwise it returns body
// unchanged. Attachments uploaded sealed travel with their keys.
func (c *Client) seal(ctx context.Context, to, typ, body string, attachments []string) (string, error) {
	s := c.Sealer
	if s == nil || s.ID == nil || !c.E2EEnabled(ctx) {
		return body, nil
	}
	var readers []api.KeyHolder
	if to == BroadcastTo {
		return body, errors.New("use sealBroadcast")
	}
	r, err := c.ReadersOf(ctx, s.Me, to)
	if err != nil {
		if e, ok := AsError(err); ok && e.Status == http.StatusNotFound {
			return body, nil
		}
		return "", err
	}
	if len(r.Missing) > 0 {
		return body, nil // an old client on one side: stays plain
	}
	readers = r.Readers
	return c.sealFor(to, typ, body, attachments, readers)
}

func (c *Client) sealFor(to, typ, body string, attachments []string, readers []api.KeyHolder) (string, error) {
	s := c.Sealer
	encs := make([]string, 0, len(readers)+1)
	for _, h := range readers {
		if err := s.pin(h); err != nil {
			return "", &Error{Code: "key_changed", Message: err.Error() + "; not sending",
				Suggestion: "if " + h.Who + " really has new keys (it was set up again), trust them: myco keys trust --who " + h.Who}
		}
		encs = append(encs, h.EncKey)
	}
	encs = append(encs, s.ID.Public().Enc)
	p := payload{Body: body}
	s.mu.Lock()
	for _, id := range attachments {
		if fk, ok := s.files[id]; ok {
			p.Files = append(p.Files, fk)
		}
	}
	s.mu.Unlock()
	j, _ := json.Marshal(p)
	return e2e.Seal(s.ID, e2e.Header{From: s.Me, To: to, Type: typ}, j, encs)
}

// sealBroadcast seals one envelope for every recipient of a broadcast.
func (c *Client) sealBroadcast(ctx context.Context, req api.BroadcastRequest) (string, error) {
	s := c.Sealer
	if s == nil || s.ID == nil || !c.E2EEnabled(ctx) {
		return req.Body, nil
	}
	targets, err := c.ListAgents(ctx, "", false, req.Selector)
	if err != nil {
		return "", err
	}
	var readers []api.KeyHolder
	for _, t := range targets {
		if t.Name == s.Me && !req.IncludeSelf {
			continue
		}
		r, err := c.ReadersOf(ctx, s.Me, t.Name)
		if err != nil {
			return "", err
		}
		if len(r.Missing) > 0 {
			return req.Body, nil
		}
		readers = append(readers, r.Readers...)
	}
	if len(readers) == 0 {
		return req.Body, nil
	}
	return c.sealFor(BroadcastTo, orType(req.Type), req.Body, req.Attachments, readers)
}

func orType(t string) string {
	if t == "" {
		return "task"
	}
	return t
}

// keysOf returns who's keys (cached for a minute).
func (c *Client) keysOf(ctx context.Context, who string) ([]api.KeyHolder, error) {
	s := c.Sealer
	s.mu.Lock()
	if k, ok := s.keys[who]; ok && time.Since(k.at) < time.Minute {
		s.mu.Unlock()
		return k.list, nil
	}
	s.mu.Unlock()
	out, err := c.KeysOf(ctx, who)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.keys == nil {
		s.keys = map[string]cachedKeys{}
	}
	s.keys[who] = cachedKeys{at: time.Now(), list: out.Keys[who]}
	s.mu.Unlock()
	return out.Keys[who], nil
}

// open decrypts a received message in place. One it cannot open keeps a
// placeholder body and says why in Unreadable.
func (c *Client) open(ctx context.Context, m *api.Message) {
	s := c.Sealer
	if s == nil || m == nil {
		return
	}
	s.remember(m)
	if !e2e.IsSealed(m.Body) {
		return
	}
	m.E2E = true
	fail := func(why string) {
		m.Unreadable = why
		m.Body = "[encrypted message: " + why + "]"
	}
	if s.ID == nil {
		fail("this client has no keys")
		return
	}
	signer := e2e.Signer(m.Body)
	keys, err := c.keysOf(ctx, m.From)
	if err != nil {
		fail("cannot look up the sender's keys: " + err.Error())
		return
	}
	var sign string
	for _, h := range keys {
		if e2e.KeyID(h.SignKey) == signer {
			if err := s.pin(h); err != nil {
				fail(err.Error() + " (trust them with: myco keys trust --who " + m.From + ")")
				return
			}
			sign = h.SignKey
			break
		}
	}
	if sign == "" {
		fail("signed with a key the hub does not list for " + m.From)
		return
	}
	to := m.To
	if m.BroadcastID != "" && m.Type != "reply" && m.Type != "event" {
		to = BroadcastTo
	}
	pt, err := e2e.Open(s.ID, e2e.Header{From: m.From, To: to, Type: m.Type}, m.Body, sign)
	if errors.Is(err, e2e.ErrNotForMe) {
		fail("only the people involved can read it")
		return
	}
	if err != nil {
		fail(err.Error())
		return
	}
	var p payload
	if json.Unmarshal(pt, &p) != nil {
		fail("malformed content")
		return
	}
	m.Body = p.Body
	if len(p.Files) > 0 {
		s.mu.Lock()
		if s.files == nil {
			s.files = map[string]fileKey{}
		}
		for _, f := range p.Files {
			s.files[f.FileID] = f
		}
		s.mu.Unlock()
		m.Attachments = m.Attachments[:0]
		for _, f := range p.Files {
			m.Attachments = append(m.Attachments, api.Attachment{FileID: f.FileID, Name: f.Name, Size: f.Size, SHA256: f.SHA256, ContentType: f.ContentType})
		}
	}
}

// openAll decrypts messages in place.
func (c *Client) openAll(ctx context.Context, ms []api.Message) {
	if c.Sealer == nil {
		return
	}
	for i := range ms {
		c.open(ctx, &ms[i])
	}
}

// Open decrypts a message in place (for callers that fetch messages
// through other routes, e.g. the app's views).
func (c *Client) Open(ctx context.Context, m *api.Message) { c.open(ctx, m) }

// uploadSealed uploads path encrypted, keeping its key for the message
// that attaches it.
func (c *Client) uploadSealed(ctx context.Context, path string) (*api.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sealed, key, err := e2e.SealBytes(data)
	if err != nil {
		return nil, err
	}
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/files"
	u.RawQuery = url.Values{"name": {"encrypted"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(sealed))
	if err != nil {
		return nil, err
	}
	req.ContentLength = int64(len(sealed))
	req.Header.Set("Content-Type", "application/octet-stream")
	var out api.File
	if err := c.send(ctx, req, &out); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	ct := mime.TypeByExtension(filepath.Ext(path))
	fk := fileKey{FileID: out.ID, Key: key, Name: filepath.Base(path), Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), ContentType: ct}
	s := c.Sealer
	s.mu.Lock()
	if s.files == nil {
		s.files = map[string]fileKey{}
	}
	s.files[out.ID] = fk
	s.mu.Unlock()
	return &api.File{ID: out.ID, Name: fk.Name, Size: fk.Size, SHA256: fk.SHA256, ContentType: ct, ExpiresAt: out.ExpiresAt}, nil
}

// maxConsoleDownload caps what the app reads into memory for one file.
const maxConsoleDownload = 100 << 20

// ConsoleDownload returns a file attached to a message the console may
// read, opened (decrypted) when it was sent encrypted and this client has
// its key from that message.
func (c *Client) ConsoleDownload(ctx context.Context, id string) (*api.File, []byte, error) {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/console/files/" + url.PathEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, nil, decodeError(resp.StatusCode, data)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxConsoleDownload+1))
	if err != nil {
		return nil, nil, &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	if len(data) > maxConsoleDownload {
		return nil, nil, &Error{Code: "too_large", Message: fmt.Sprintf("file %s is larger than %d bytes", id, maxConsoleDownload)}
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if want := resp.Header.Get("X-Myco-SHA256"); want != "" && want != got {
		return nil, nil, &Error{Code: "integrity_error", Message: fmt.Sprintf("checksum mismatch for %s", id), Retryable: true}
	}
	if fk, ok := c.fileKeyOf(id); ok {
		// An encrypted attachment: its key came in the message.
		plain, err := e2e.OpenBytes(data, fk.Key)
		if err != nil {
			return nil, nil, &Error{Code: "integrity_error", Message: "cannot decrypt attachment " + fk.Name}
		}
		ps := sha256.Sum256(plain)
		if hex.EncodeToString(ps[:]) != fk.SHA256 {
			return nil, nil, &Error{Code: "integrity_error", Message: fmt.Sprintf("checksum mismatch for %s", fk.Name)}
		}
		return &api.File{ID: id, Name: fk.Name, Size: int64(len(plain)), SHA256: fk.SHA256, ContentType: fk.ContentType}, plain, nil
	}
	name, _ := url.PathUnescape(resp.Header.Get("X-Myco-File-Name"))
	if name == "" {
		name = id
	}
	return &api.File{ID: id, Name: name, Size: int64(len(data)), SHA256: got, ContentType: resp.Header.Get("Content-Type")}, data, nil
}

// fileKeyOf returns the key of an encrypted attachment this client has seen.
func (c *Client) fileKeyOf(id string) (fileKey, bool) {
	s := c.Sealer
	if s == nil {
		return fileKey{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fk, ok := s.files[id]
	return fk, ok
}

// openDownloaded decrypts a downloaded encrypted attachment in place and
// renames it to its real name.
func openDownloaded(fk fileKey, path string) (string, error) {
	sealed, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	data, err := e2e.OpenBytes(sealed, fk.Key)
	if err != nil {
		return "", &Error{Code: "integrity_error", Message: "cannot decrypt attachment " + fk.Name}
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != fk.SHA256 {
		return "", &Error{Code: "integrity_error", Message: fmt.Sprintf("checksum mismatch for %s", fk.Name)}
	}
	dest := path
	if filepath.Base(path) == "encrypted" || strings.HasPrefix(filepath.Base(path), "encrypted") {
		dest = filepath.Join(filepath.Dir(path), filepath.Base(fk.Name))
	}
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", err
	}
	if dest != path {
		_ = os.Remove(path)
	}
	return dest, nil
}

// sealAbout seals body for a message about message id: answers (reply,
// progress, failure) go back to its sender; a correction (toRecipient) goes
// to its recipient.
func (c *Client) sealAbout(ctx context.Context, id string, toRecipient bool, typ, body string, attachments []string) (string, error) {
	if c.Sealer == nil || c.Sealer.ID == nil || !c.E2EEnabled(ctx) {
		return body, nil
	}
	p, err := c.partiesOf(ctx, id)
	if err != nil {
		return "", err
	}
	to := p.from
	if toRecipient {
		to = p.to
	}
	return c.seal(ctx, to, typ, body, attachments)
}

// ConsoleUpload stores data as a file the console (the app) attaches to
// what it sends next (POST /v1/console/files). With end-to-end encryption
// on, it is sealed first and its key kept, as UploadFile does, so the next
// ConsoleSend that attaches it carries the key to the recipient.
func (c *Client) ConsoleUpload(ctx context.Context, name, contentType string, data []byte) (*api.File, error) {
	body, fileName, ct := data, name, contentType
	var key string
	sealed := c.Sealer != nil && c.Sealer.ID != nil && c.E2EEnabled(ctx)
	if sealed {
		var err error
		if body, key, err = e2e.SealBytes(data); err != nil {
			return nil, err
		}
		fileName, ct = "encrypted", "application/octet-stream"
	}
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/console/files"
	u.RawQuery = url.Values{"name": {fileName}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.ContentLength = int64(len(body))
	if ct == "" {
		ct = "application/octet-stream"
	}
	req.Header.Set("Content-Type", ct)
	var out api.File
	if err := c.send(ctx, req, &out); err != nil {
		return nil, err
	}
	if !sealed {
		return &out, nil
	}
	sum := sha256.Sum256(data)
	fk := fileKey{FileID: out.ID, Key: key, Name: name, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), ContentType: contentType}
	s := c.Sealer
	s.mu.Lock()
	if s.files == nil {
		s.files = map[string]fileKey{}
	}
	s.files[out.ID] = fk
	s.mu.Unlock()
	return &api.File{ID: out.ID, Name: name, Size: fk.Size, SHA256: fk.SHA256, ContentType: contentType, ExpiresAt: out.ExpiresAt}, nil
}
