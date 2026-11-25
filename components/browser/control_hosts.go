package main

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"golang.org/x/net/idna"
)

func normalizeControlHosts(hosts []string) ([]string, error) {
	if len(hosts) > 64 {
		return nil, errors.New("invalid_argument")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range hosts {
		wild := strings.HasPrefix(raw, "*.")
		name := raw
		if wild {
			name = raw[2:]
		}
		name = strings.TrimSuffix(strings.ToLower(name), ".")
		ascii, err := idna.Lookup.ToASCII(name)
		if err != nil || ascii == "" || len(ascii) > 253 || strings.ContainsAny(ascii, "/\\@:#?[] *\t\r\n") {
			return nil, errors.New("invalid_argument")
		}
		if net.ParseIP(ascii) == nil {
			for _, label := range strings.Split(ascii, ".") {
				if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
					return nil, errors.New("invalid_argument")
				}
			}
		} else if wild {
			return nil, errors.New("invalid_argument")
		}
		if wild {
			ascii = "*." + ascii
		}
		if !seen[ascii] {
			seen[ascii] = true
			out = append(out, ascii)
		}
	}
	return out, nil
}
func controlHostAllowed(hosts []string, raw string) bool {
	if len(hosts) == 0 || raw == "about:blank" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	name, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."))
	if err != nil {
		return false
	}
	for _, pattern := range hosts {
		if name == pattern {
			return true
		}
		if strings.HasPrefix(pattern, "*.") && strings.HasSuffix(name, pattern[1:]) && name != pattern[2:] {
			return true
		}
	}
	return false
}
func (c *controlSession) hostViolation(raw string) *controlError {
	host := ""
	if u, err := url.Parse(raw); err == nil {
		host = u.Hostname()
	}
	x := controlFailure("host_not_allowed", "")
	x.Host = truncate(host, 253)
	return x
}
func (c *controlSession) recordHostViolation(x *controlError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.violation = x
	c.events = append(c.events, controlEvent{Host: x.Host, Action: "host_not_allowed", Code: "host_not_allowed", Time: time.Now().UTC()})
	close(c.wake)
	c.wake = make(chan struct{})
}

// Domain scope applies to top-level pages only. Iframes and subresources are
// deliberately untouched. All correction uses a bounded native CDP context.
func (c *controlSession) enforceHosts(ctx context.Context) *controlError {
	if len(c.hosts) == 0 {
		return nil
	}
	c.hostMu.Lock()
	defer c.hostMu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithValue(c.ctx, engineInternal{}, true), 3*time.Second)
	defer cancel()
	c.browser.mu.Lock()
	pages := []*rod.Page{}
	for _, p := range c.browser.pages {
		pages = append(pages, p)
	}
	c.browser.mu.Unlock()
	var denied *controlError
	for _, p := range pages {
		info, err := p.Context(ctx).Info()
		if err != nil {
			continue
		}
		// An attached target may have no committed document during startup. It
		// has not navigated anywhere yet; explicit navigation is preflighted.
		if info.URL == "" || controlHostAllowed(c.hosts, info.URL) {
			continue
		}
		x := c.hostViolation(info.URL)
		denied = x
		c.recordHostViolation(x)
		c.mu.Lock()
		previous := c.allowedURLs[p.TargetID]
		if c.hostRecovered == nil {
			c.hostRecovered = map[proto.TargetTargetID]bool{}
		}
		alreadyRecovered := c.hostRecovered[p.TargetID]
		c.hostRecovered[p.TargetID] = true
		c.mu.Unlock()
		if p.TargetID != c.initialTab && previous == "" {
			_ = p.Context(ctx).Close()
			continue
		}
		if alreadyRecovered {
			if p.Context(ctx).Navigate("about:blank") != nil {
				_ = p.Context(ctx).Close()
			}
			continue
		}
		sid := c.browser.primarySession(p.TargetID)
		var history proto.PageGetNavigationHistoryResult
		restored := false
		if c.call(ctx, sid, "Page.getNavigationHistory", nil, &history) == nil {
			for i := history.CurrentIndex; i >= 0; i-- {
				if controlHostAllowed(c.hosts, history.Entries[i].URL) && (previous == "" || history.Entries[i].URL != "about:blank") {
					if c.call(ctx, sid, "Page.navigateToHistoryEntry", map[string]any{"entryId": history.Entries[i].ID}, nil) == nil {
						restored = true
					}
					break
				}
			}
		}
		if !restored && previous != "" {
			restored = p.Context(ctx).Navigate(previous) == nil
		}
		if restored {
			restored = c.waitAllowed(ctx, p)
		}
		if !restored {
			if p.Context(ctx).Navigate("about:blank") != nil {
				_ = p.Context(ctx).Close()
			}
		}
	}
	return denied
}
func (c *controlSession) enforceHostsSoon() {
	if len(c.hosts) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithValue(c.ctx, engineInternal{}, true), 3*time.Second)
		defer cancel()
		c.enforceHosts(ctx)
	}()
}
func (c *controlSession) takeHostViolation() *controlError {
	c.mu.Lock()
	defer c.mu.Unlock()
	x := c.violation
	c.violation = nil
	return x
}

func (c *controlSession) hasHostViolation() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.violation != nil
}

func (c *controlSession) waitAllowed(ctx context.Context, p *rod.Page) bool {
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		if info, err := p.Context(ctx).Info(); err == nil && controlHostAllowed(c.hosts, info.URL) {
			return c.waitReady(ctx, p) == nil
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-time.After(10 * time.Millisecond):
		}
	}
}
