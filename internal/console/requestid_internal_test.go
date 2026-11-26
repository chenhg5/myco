package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/requestid"
)

// One request of the App keeps one id through its backend to the hub: the
// App's own, or one the backend makes; answered back to the App.
func TestRequestIDReachesTheHub(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get(requestid.Header))
		mu.Unlock()
		json.NewEncoder(w).Encode(api.Exchange{Messages: []api.Message{}})
	}))
	defer hub.Close()
	home := t.TempDir()
	store := NewStore(filepath.Join(home, "hubs.json"))
	saved, err := store.Add("Fixture", hub.URL)
	if err != nil {
		t.Fatal(err)
	}
	b := &Backend{Local: &Local{Home: home}, Store: store, Secrets: browserFixtureSecrets{}}
	h := b.Handler()
	// (the ids the hub got for one request of the App: every call made for it)
	get := func(id string) (string, []string) {
		mu.Lock()
		seen = nil
		mu.Unlock()
		req := httptest.NewRequest("GET", "http://app/app/hubs/"+saved.ID+"/exchanges?a=x&b=y", nil)
		if id != "" {
			req.Header.Set(requestid.Header, id)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("exchanges: %d %s", rec.Code, rec.Body.String())
		}
		mu.Lock()
		defer mu.Unlock()
		return rec.Header().Get(requestid.Header), append([]string(nil), seen...)
	}
	same := func(ids []string, want string) bool {
		for _, x := range ids {
			if x != want {
				return false
			}
		}
		return len(ids) > 0
	}
	if got, hubGot := get("app-123"); got != "app-123" || !same(hubGot, "app-123") {
		t.Fatalf("the App's id is answered back and sent to the hub: %q, hub got %q", got, hubGot)
	}
	if made, hubGot := get(""); requestid.Valid(made) == "" || !same(hubGot, made) {
		t.Fatalf("the backend makes one and sends it on: %q, hub got %q", made, hubGot)
	}
	if got, hubGot := get("bad id\n"); requestid.Valid(got) == "" || !same(hubGot, got) {
		t.Fatalf("an id that is not one is replaced: %q, hub got %q", got, hubGot)
	}
}
