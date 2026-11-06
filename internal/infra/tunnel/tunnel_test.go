package tunnel

import (
	"strings"
	"testing"
)

func TestQuickTunnelURL(t *testing.T) {
	cases := map[string]string{
		"2026-11-06T01:02:03Z INF |  https://smart-fox-river-abc.trycloudflare.com                       |": "https://smart-fox-river-abc.trycloudflare.com",
		"INF Requesting new quick Tunnel on trycloudflare.com...":                                           "",
		"ERR ... https://api.trycloudflare.com/tunnel: 500":                                                 "",
	}
	for line, want := range cases {
		if got := quickTunnelURL(line); got != want {
			t.Errorf("%q: got %q want %q", line, got, want)
		}
	}
}

func TestTunnelIDFromList(t *testing.T) {
	out := `[{"id":"old","name":"myco","deleted_at":"2026-01-01T00:00:00Z"},{"id":"6ff4-1","name":"myco","deleted_at":"0001-01-01T00:00:00Z"},{"id":"x","name":"other"}]`
	if id, err := tunnelIDFromList(out, "myco"); err != nil || id != "6ff4-1" {
		t.Fatalf("got %q %v", id, err)
	}
	if id, _ := tunnelIDFromList("[]", "myco"); id != "" {
		t.Fatalf("missing tunnel: %q", id)
	}
	if _, err := tunnelIDFromList("not json", "myco"); err == nil {
		t.Fatal("bad output must be an error")
	}
}

func TestCloudflaredConfig(t *testing.T) {
	c := cloudflaredConfig("6ff4-1", "/h/.cloudflared/6ff4-1.json", "hub.example.com", "http://127.0.0.1:7777/")
	for _, want := range []string{"tunnel: 6ff4-1", "credentials-file: /h/.cloudflared/6ff4-1.json", "hostname: hub.example.com", "service: http://127.0.0.1:7777\n", "http_status:404"} {
		if !strings.Contains(c, want) {
			t.Errorf("config lacks %q:\n%s", want, c)
		}
	}
}

func TestTailscaleStatus(t *testing.T) {
	state, dns := tailscaleStatus(`{"BackendState":"Running","Self":{"DNSName":"glenn-mac.tail1234.ts.net.","HostName":"glenn-mac"}}`)
	if state != "Running" || dns != "glenn-mac.tail1234.ts.net" {
		t.Fatalf("got %q %q", state, dns)
	}
	if state, _ := tailscaleStatus(`{"BackendState":"NeedsLogin","Self":{"DNSName":""}}`); state != "NeedsLogin" {
		t.Fatalf("state %q", state)
	}
}

func TestNew(t *testing.T) {
	if _, err := New(Options{Kind: "ngrok"}); err == nil {
		t.Fatal("unknown tunnel kinds must be rejected")
	}
	c, err := New(Options{Kind: "cloudflare"})
	if err != nil || c.(*Cloudflare).o.Name != "myco" {
		t.Fatalf("default tunnel name: %v", err)
	}
}
