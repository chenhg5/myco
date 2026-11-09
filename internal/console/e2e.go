package console

import (
	"context"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/e2e"
)

// The app reads and writes end-to-end encrypted messages as this device:
// each hub gets its own key pair, kept in the keychain next to the device's
// console token. Decryption happens here, in the app's backend; the page
// only ever sees plain text.

// e2eKeyName is the keychain entry of a hub's device key pair.
func e2eKeyName(hubID string) string { return hubID + ".e2e" }

type sealers struct {
	mu sync.Mutex
	m  map[string]*sealerEntry
}

type sealerEntry struct {
	s       *client.Sealer
	checked time.Time // when the member name was last looked up
}

// withSealer attaches this device's sealer for hub h to c (once the hub
// has said who this device belongs to).
func (b *Backend) withSealer(ctx context.Context, hubID string, c *client.Client) {
	b.sealers.mu.Lock()
	if b.sealers.m == nil {
		b.sealers.m = map[string]*sealerEntry{}
	}
	e := b.sealers.m[hubID]
	if e != nil && e.s != nil {
		b.sealers.mu.Unlock()
		c.Sealer = e.s
		return
	}
	if e != nil && time.Since(e.checked) < time.Minute {
		b.sealers.mu.Unlock()
		return // an older hub, or no member: retried later
	}
	b.sealers.mu.Unlock()

	entry := &sealerEntry{checked: time.Now()}
	defer func() {
		b.sealers.mu.Lock()
		b.sealers.m[hubID] = entry
		b.sealers.mu.Unlock()
	}()
	if b.Secrets == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	who, err := c.Whoami(cctx)
	if err != nil || who.Member == nil || who.Device == nil {
		return
	}
	id, err := e2e.ParseIdentity(b.secret(e2eKeyName(hubID)))
	if err != nil {
		if id, err = e2e.NewIdentity(); err != nil {
			return
		}
		if err := b.Secrets.Set(e2eKeyName(hubID), id.Marshal()); err != nil {
			return
		}
	}
	entry.s = &client.Sealer{Me: "@console:" + who.Member.Name, ID: id, Device: true}
	c.Sealer = entry.s
	_ = c.RegisterKeys(cctx)
}

func (b *Backend) secret(name string) string {
	v, err := b.Secrets.Get(name)
	if err != nil {
		return ""
	}
	return v
}

// forgetSealer drops a hub's device keys (when the hub is removed).
func (b *Backend) forgetSealer(hubID string) {
	b.sealers.mu.Lock()
	delete(b.sealers.m, hubID)
	b.sealers.mu.Unlock()
	if b.Secrets != nil {
		_ = b.Secrets.Delete(e2eKeyName(hubID))
	}
}
