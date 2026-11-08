package console

import (
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/chenhg5/myco/internal/app"
)

// ParseAppLink reads an app link — https://<hub>/c/<console token>, or
// myco://add-hub?link=<that link> — into the hub's address and token.
func ParseAppLink(raw string) (hubURL, token string, err error) {
	raw = strings.TrimSpace(raw)
	if u, perr := url.Parse(raw); perr == nil && u.Scheme == "myco" {
		raw = u.Query().Get("link")
	}
	u, perr := url.Parse(raw)
	if perr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", "", errors.New("这不是 App 连接链接：应该形如 https://你的服务器/c/myco_console_…")
	}
	base, tok, ok := strings.Cut(u.EscapedPath(), "/c/")
	tok = strings.Trim(tok, "/")
	if !ok || !strings.HasPrefix(tok, app.ConsoleTokenPrefix) || strings.Contains(tok, "/") {
		return "", "", errors.New("这不是 App 连接链接：应该形如 https://你的服务器/c/myco_console_…")
	}
	if u.User != nil {
		return "", "", errors.New("the link must not contain a user name or password")
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(base, "/"), tok, nil
}

// pendingLink holds a link the app was opened with (myco://…) until the
// UI picks it up.
type pendingLink struct {
	mu   sync.Mutex
	link string
}

// SetPendingLink records a link the app was opened with; the UI reads it
// once through GET /app/pending-link and offers to add the hub.
func (b *Backend) SetPendingLink(link string) {
	b.pending.mu.Lock()
	b.pending.link = link
	b.pending.mu.Unlock()
}

func (b *Backend) takePendingLink() string {
	b.pending.mu.Lock()
	defer b.pending.mu.Unlock()
	l := b.pending.link
	b.pending.link = ""
	if l == "" {
		return ""
	}
	// Hand the UI the plain https link (that is what it shows and pastes).
	if u, err := url.Parse(l); err == nil && u.Scheme == "myco" {
		return u.Query().Get("link")
	}
	return l
}
