package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// Keep official host routing in the production code while all test traffic is
// handled by a local server. No caller environment or real credentials are used.
type modelTestRoute struct {
	target    *url.URL
	transport http.RoundTripper
}

func (t modelTestRoute) RoundTrip(r *http.Request) (*http.Response, error) {
	q := r.Clone(r.Context())
	u := *r.URL
	q.URL = &u
	q.Host = r.URL.Host
	q.URL.Scheme = t.target.Scheme
	q.URL.Host = t.target.Host
	return t.transport.RoundTrip(q)
}
func modelTestClient(s *httptest.Server) *http.Client {
	u, _ := url.Parse(s.URL)
	return &http.Client{Transport: modelTestRoute{u, s.Client().Transport}}
}

func TestMiniMaxRegionsAndMissingModelLists(t *testing.T) {
	for _, host := range []string{"api.minimax.io", "api.minimaxi.com"} {
		for _, anthropic := range []bool{true, false} {
			name := host + "/openai"
			if anthropic {
				name = host + "/anthropic"
			}
			t.Run(name, func(t *testing.T) {
				var posts atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Host != host {
						t.Errorf("key sent to another region: %s", r.Host)
					}
					if r.Header.Get("Authorization") != "Bearer "+testKey {
						t.Error("missing authentication")
					}
					if r.Method == http.MethodGet {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					posts.Add(1)
					if host == "api.minimax.io" {
						http.Error(w, testKey, http.StatusUnauthorized)
						return
					}
					want := "/v1/chat/completions"
					if anthropic {
						want = "/anthropic/v1/messages"
					}
					if r.URL.Path != want {
						t.Errorf("completion path %s", r.URL.Path)
					}
					var body struct {
						Model     string `json:"model"`
						MaxTokens int    `json:"max_tokens"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "MiniMax-M3" || body.MaxTokens != 16 {
						t.Errorf("unbounded or incorrect test: %+v", body)
					}
					if anthropic {
						w.Write([]byte(`{"id":"msg_fixture","type":"message","role":"assistant","content":[]}`))
					} else {
						w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Hi"}}]}`))
					}
				}))
				defer srv.Close()
				base := "https://" + host + "/v1"
				if anthropic {
					base = "https://" + host + "/anthropic"
				}
				got, err := TestModels(context.Background(), modelTestClient(srv), Endpoint{Base: base, Anthropic: anthropic, Key: testKey})
				if posts.Load() != 1 {
					t.Fatalf("completion attempts = %d", posts.Load())
				}
				if host == "api.minimax.io" {
					var status *HTTPStatusError
					if !errors.As(err, &status) || status.Status != 401 || status.Host != host || strings.Contains(err.Error(), testKey) {
						t.Fatalf("unsafe or missing rejection: %v", err)
					}
				} else if err != nil || got.From != "catalog" || len(got.Models) != 2 || got.Models[0].ID != "MiniMax-M3" {
					t.Fatalf("verified presets = %+v, %v", got, err)
				}
			})
		}
	}
}

func TestMiniMaxModelListDoesNotGenerateCompletions(t *testing.T) {
	for _, host := range []string{"api.minimax.io", "api.minimaxi.com"} {
		t.Run(host, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("a working model list generated a paid request")
				}
				w.Write([]byte(`{"data":[{"id":"model-from-account"}]}`))
			}))
			defer srv.Close()
			got, err := TestModels(context.Background(), modelTestClient(srv), Endpoint{Base: "https://" + host + "/anthropic", Anthropic: true, Key: testKey})
			if err != nil || got.From != "provider" || len(got.Models) != 1 || got.Models[0].ID != "model-from-account" {
				t.Fatalf("live list: %+v, %v", got, err)
			}
		})
	}
}

func TestMiniMaxFallbackRefusesErrorsAndInvalidSuccess(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.WriteHeader(404)
					return
				}
				w.WriteHeader(status)
				w.Write([]byte(`{"error":"` + testKey + `"}`))
			}))
			defer srv.Close()
			got, err := TestModels(context.Background(), modelTestClient(srv), Endpoint{Base: "https://api.minimaxi.com/anthropic", Anthropic: true, Key: testKey})
			if err == nil || len(got.Models) != 0 || strings.Contains(err.Error(), testKey) {
				t.Fatalf("accepted API error or leaked response: %+v, %v", got, err)
			}
		})
	}
}

func TestModelTestDoesNotProbeOtherVendorsOrTemporaryFailures(t *testing.T) {
	for _, base := range []string{"https://api.deepseek.com/anthropic", "https://custom.example/v1", "https://api.minimaxi.com/anthropic"} {
		t.Run(base, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("unexpected completion")
				}
				if strings.Contains(base, "minimaxi.com") {
					w.WriteHeader(429)
				} else {
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			if _, err := TestModels(context.Background(), modelTestClient(srv), Endpoint{Base: base, Anthropic: true, Key: testKey}); err == nil {
				t.Fatal("unverified provider accepted")
			}
		})
	}
}

func TestMiniMaxCompletionRedirectDoesNotSendKeyElsewhere(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(404)
			return
		}
		http.Redirect(w, r, other.URL+"/messages", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	if _, err := TestModels(context.Background(), modelTestClient(srv), Endpoint{Base: "https://api.minimaxi.com/anthropic", Anthropic: true, Key: testKey}); err == nil || leaked.Load() {
		t.Fatalf("redirect accepted or key sent elsewhere: %v", err)
	}
}

func TestMiniMaxMixedListFailuresDoNotGenerateCompletions(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
				}
				if r.URL.Path == "/anthropic/v1/models" {
					w.WriteHeader(status)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()
			if _, err := TestModels(context.Background(), modelTestClient(srv), Endpoint{Base: "https://api.minimaxi.com/anthropic", Anthropic: true, Key: testKey}); err == nil || posts.Load() != 0 {
				t.Fatalf("temporary failure triggered generation: %v; calls %d", err, posts.Load())
			}
		})
	}
}

func TestMiniMaxRejectsNullCompletionContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"id":"msg_fixture","type":"message","role":"assistant","content":null}`))
	}))
	defer srv.Close()
	if _, err := TestModels(context.Background(), modelTestClient(srv), Endpoint{Base: "https://api.minimaxi.com/anthropic", Anthropic: true, Key: testKey}); err == nil {
		t.Fatal("malformed completion accepted")
	}
}
