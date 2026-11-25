package client

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPageAttachmentOldHubDoesNotUploadProjectFiles(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/v1/pages/pg_demo" {
			t.Errorf("unexpected fallback: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"pg_demo","name":"Assets"}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "test")
	_, err := c.PutPageAttachment(context.Background(), PageAttachmentUpload{Page: "pg_demo", Collection: "assets", Name: "image.png", Key: "key"}, bytes.NewReader([]byte{1}), 1)
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "unsupported" || calls != 1 {
		t.Fatalf("old Hub capability: %v; calls %d", err, calls)
	}
}

func TestPageAttachmentCapabilityPreservesPageErrors(t *testing.T) {
	for _, status := range []int{401, 403, 404} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte(`{"error":"not_found","message":"page unavailable"}`))
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "test")
			_, err := c.PageAttachments(context.Background(), "pg_demo", "", 100)
			var ce *Error
			if !errors.As(err, &ce) || ce.Status != status {
				t.Fatalf("page error changed: %v", err)
			}
		})
	}
}
