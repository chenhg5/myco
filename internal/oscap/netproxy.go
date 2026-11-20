package oscap

import (
	"context"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Downloads (speech.install) go through the proxy the App or the
// environment gives, and otherwise through the one set in the system's
// network settings (macOS, Windows): a GUI app's environment has none, and
// in some places GitHub is reachable only through it. If that proxy is not
// there, they go direct.

var (
	sysProxyOnce sync.Once
	sysProxyURL  *url.URL
)

func systemProxy() *url.URL {
	sysProxyOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		switch runtime.GOOS {
		case "darwin":
			if out, err := exec.CommandContext(ctx, "scutil", "--proxy").Output(); err == nil {
				sysProxyURL = scutilProxy(string(out))
			}
		case "windows":
			c := exec.CommandContext(ctx, "reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
			hideWindow(c)
			if out, err := c.Output(); err == nil {
				sysProxyURL = winProxy(string(out))
			}
		}
	})
	return sysProxyURL
}

// scutilProxy reads `scutil --proxy`: the HTTPS proxy, when it is on.
func scutilProxy(s string) *url.URL {
	v := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		k, val, ok := strings.Cut(line, ":")
		if ok {
			v[strings.TrimSpace(k)] = strings.TrimSpace(val)
		}
	}
	for _, p := range []string{"HTTPS", "HTTP"} {
		if v[p+"Enable"] == "1" && v[p+"Proxy"] != "" && v[p+"Port"] != "" {
			return &url.URL{Scheme: "http", Host: v[p+"Proxy"] + ":" + v[p+"Port"]}
		}
	}
	return nil
}

// winProxy reads Internet Settings (reg query): ProxyServer, when ProxyEnable.
func winProxy(s string) *url.URL {
	enabled, server := false, ""
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		switch f[0] {
		case "ProxyEnable":
			enabled = f[len(f)-1] == "0x1"
		case "ProxyServer":
			server = f[len(f)-1]
		}
	}
	if !enabled || server == "" {
		return nil
	}
	// "host:port", or per scheme: "http=h:p;https=h:p"
	if strings.Contains(server, "=") {
		pick := ""
		for _, part := range strings.Split(server, ";") {
			k, v, _ := strings.Cut(part, "=")
			if k == "https" || (k == "http" && pick == "") {
				pick = v
			}
		}
		server = pick
	}
	if server == "" {
		return nil
	}
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}
	u, err := url.Parse(server)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}

// downloadClients: the ways to try, in order.
func downloadClients() []*http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return []*http.Client{http.DefaultClient}
	}
	sys := systemProxy()
	if sys == nil {
		return []*http.Client{http.DefaultClient}
	}
	viaSys := base.Clone()
	viaSys.Proxy = func(r *http.Request) (*url.URL, error) {
		if base.Proxy != nil {
			if u, err := base.Proxy(r); u != nil || err != nil {
				return u, err
			}
		}
		return sys, nil
	}
	return []*http.Client{{Transport: viaSys}, http.DefaultClient}
}
