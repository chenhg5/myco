package console

import (
	"net"
	"net/url"
	"strings"

	"github.com/myco-ai/myco/internal/osutil"
)

// hubAliases are the other addresses a link to this hub can carry. A hub
// on this computer that the App reaches at 127.0.0.1 writes its links
// (pages, files) with this computer's LAN address when it serves the local
// network, so that people on other machines can open them: the App must
// still know those links are this hub's.
func hubAliases(raw string) []string {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Host == "" {
		return nil
	}
	host, port := u.Hostname(), u.Port()
	if !isLoopbackName(host) {
		return nil
	}
	hosts := []string{"127.0.0.1", "localhost", "::1"}
	if ip := osutil.LANAddr(); ip != "" {
		hosts = append(hosts, ip)
	}
	var out []string
	for _, h := range hosts {
		if h == host {
			continue
		}
		v := *u
		if port != "" {
			v.Host = net.JoinHostPort(h, port)
		} else if strings.Contains(h, ":") {
			v.Host = "[" + h + "]"
		} else {
			v.Host = h
		}
		out = append(out, v.String())
	}
	return out
}

func isLoopbackName(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
