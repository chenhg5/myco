package main

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

func TestNativeChromePassiveEvidenceRetainsNestedTargetAndPrivacy(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/outer":
			fmt.Fprint(w, `<!doctype html><iframe id="inner" src="/target" style="width:400px;height:200px"></iframe>`)
		case "/target":
			fmt.Fprint(w, `<!doctype html><div id="host"></div><script>document.querySelector('#host').attachShadow({mode:'open'}).innerHTML='<button id="go" style="width:140px;height:40px;border:0;background:rgb(0,220,0)">Evidence</button>'</script>`)
		default:
			fmt.Fprint(w, `<!doctype html><iframe id="outer" src="/outer" style="width:500px;height:300px"></iframe>`)
		}
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	e.profiles["p_evidence"] = profile{ID: "p_evidence", Name: "Evidence fixture"}
	s, err := e.openSession(ctx, "native/evidence", "p_evidence", true)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	p = p.Context(ctx)
	if err = p.Navigate(fixture.URL); err != nil {
		t.Fatal(err)
	}
	if err = p.WaitLoad(); err != nil {
		t.Fatal(err)
	}
	target := target{Frame: []string{"#outer", "#inner"}, Shadow: []string{"#host"}, Locators: []locator{{Kind: "css", Value: "#go"}}}
	id, err := e.snapshot("native/evidence", p, &target)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(e.dir, "artifacts", id))
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	ratio := float64(bounds.Dx()) / float64(bounds.Dy())
	r, g, b, _ := img.At(bounds.Min.X+5, bounds.Min.Y+5).RGBA()
	if ratio < 3.4 || ratio > 3.6 || g < 40000 || r > 20000 || b > 20000 {
		t.Fatal("target crop was lost", bounds, r, g, b)
	}
	_, err = (proto.RuntimeEvaluate{Expression: `document.querySelector('#outer').contentDocument.querySelector('#inner').contentDocument.querySelector('#host').shadowRoot.innerHTML='<input type="password">'`}).Call(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.snapshot("native/evidence", p, &target); err == nil || err.Error() != "sensitive_snapshot" {
		t.Fatal("sensitive evidence was accepted", err)
	}
}
