package console_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

func TestPageAttachmentProxyRangeAndReferenceLifecycle(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "maker"}, false)
	if err != nil {
		t.Fatal(err)
	}
	maker, _ := client.New(r.hubURL, reg.Token)
	var bundle bytes.Buffer
	z := zip.NewWriter(&bundle)
	for name, body := range map[string]string{"index.html": "<h1>Assets</h1>", "myco.json": `{"id":"demo/assets","name":"Assets","version":"1.0.0","data":{"assets":{"fields":{"attachment":"file"}}}}`} {
		f, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write([]byte(body)); e != nil {
			t.Fatal(e)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	template, err := maker.PublishTemplate(ctx, bytes.NewReader(bundle.Bytes()), int64(bundle.Len()))
	if err != nil {
		t.Fatal(err)
	}
	used, err := maker.UseTemplate(ctx, template.ID, api.TemplateUse{})
	if err != nil {
		t.Fatal(err)
	}
	code, hub, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Demo", "url": r.hubURL, "token": r.consoleToken("page-files")})
	if code != 201 {
		t.Fatal(code, hub)
	}
	path := "/app/hubs/" + hub["id"].(string) + "/pages/" + used.Page.ID + "/attachments"
	request := func(method, path string, body []byte, headers map[string]string) (int, http.Header, []byte) {
		t.Helper()
		req, e := http.NewRequest(method, r.app.URL+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		response, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, response.Header, data
	}
	content := bytes.Repeat([]byte{0, 1, 2, 3}, 1<<18)
	code, _, raw := request("POST", path+"?collection=assets&name=sample.bin", content, map[string]string{"Content-Type": "application/octet-stream", "Idempotency-Key": "proxy-upload"})
	var file api.PageAttachment
	if code != 201 || json.Unmarshal(raw, &file) != nil {
		t.Fatalf("upload: %d %s", code, raw)
	}
	url := path + "/" + file.ID
	code, headers, raw := request("GET", url+"/content", nil, map[string]string{"Range": "bytes=5-12"})
	if code != 206 || !bytes.Equal(raw, content[5:13]) || headers.Get("Content-Range") != fmt.Sprintf("bytes 5-12/%d", len(content)) || headers.Get("Accept-Ranges") != "bytes" {
		t.Fatal("proxy lost Range", code, headers)
	}
	code, headers, _ = request("GET", url+"/content", nil, map[string]string{"Range": "bytes=999999999-"})
	if code != 416 || headers.Get("Content-Range") != fmt.Sprintf("bytes */%d", len(content)) {
		t.Fatal("proxy lost 416", code, headers)
	}
	zero := 0
	if _, err = r.admin.PutPageRecord(ctx, used.Page.ID, "assets", "sample", json.RawMessage(fmt.Sprintf(`{"attachment":{"file_id":%q}}`, file.ID)), &zero); err != nil {
		t.Fatal(err)
	}
	if code, _, raw = request("DELETE", url, nil, nil); code != 409 {
		t.Fatalf("removed referenced file: %d %s", code, raw)
	}
	if err = r.admin.DeletePageRecord(ctx, used.Page.ID, "assets", "sample", 1); err != nil {
		t.Fatal(err)
	}
	if code, _, raw = request("DELETE", url, nil, nil); code != 204 {
		t.Fatalf("remove: %d %s", code, raw)
	}
	if code, _, _ = request("GET", url+"/content", nil, nil); code != 404 {
		t.Fatal("deleted content survived", code)
	}
	code, _, raw = request("GET", path, nil, nil)
	var list api.PageAttachmentList
	if code != 200 || json.Unmarshal(raw, &list) != nil || list.Usage.FilesUsed != 0 {
		t.Fatalf("usage: %d %s", code, raw)
	}
}
