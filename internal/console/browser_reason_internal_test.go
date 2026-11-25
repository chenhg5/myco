package console

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/myco-ai/myco/internal/browserautomation"
)

func TestBrowserProfileRecoveryReasonReachesPageError(t *testing.T) {
	for _, reason := range []string{"leftover_unverified", "cleanup_pending", "cleanup_failed", "private/process/path"} {
		w := httptest.NewRecorder()
		writeErr(w, browserComponentFailure(&browserautomation.ProfileError{Reason: reason}))
		var body map[string]string
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 409 || body["error"] != "profile_locked" {
			t.Fatal(w.Code, w.Body.String())
		}
		want := reason
		if reason == "private/process/path" {
			want = "leftover_unverified"
		}
		if body["reason"] != want {
			t.Fatal(body)
		}
	}
}
