package client

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The tunnel transport speaks HTTP/1.1 also to an HTTPS server that offers
// HTTP/2 (Caddy in front of a hub): offered h2 in the handshake, such a
// server answers in it, and an HTTP/1.1 client cannot read that.
func TestTunnelTransportStaysOnHTTP1OverTLS(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Proto)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	tr := TunnelTransport().(*http.Transport).Clone()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	tr.TLSClientConfig = tr.TLSClientConfig.Clone()
	tr.TLSClientConfig.RootCAs = pool
	tr.Proxy = nil
	if got := tr.TLSClientConfig.NextProtos; len(got) != 1 || got[0] != "http/1.1" {
		t.Fatalf("offers %v", got)
	}
	res, err := (&http.Client{Transport: tr}).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.Proto != "HTTP/1.1" || string(b) != "HTTP/1.1" || res.TLS == nil || res.TLS.NegotiatedProtocol == "h2" {
		t.Fatalf("got %s / %s (%v)", res.Proto, b, res.TLS.NegotiatedProtocol)
	}
	_ = tls.VersionTLS12
}
