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

func TestBrowserOwnerWindowBusyReasonReachesAgentAndPage(t *testing.T) {
	err := &browserautomation.ComponentError{Code: "busy", Reason: "owner_window_open"}
	out := browserControlFailure(err, false)
	if out.Error.Reason != "owner_window_open" || out.Error.Message != "Close this profile's browser window before opening agent control." {
		t.Fatal(out)
	}
	w := httptest.NewRecorder()
	writeErr(w, browserComponentFailure(err))
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["reason"] != "owner_window_open" || body["error"] != "busy" {
		t.Fatal(body)
	}
}

func TestBrowserScreenshotPrivacyReasonReachesAgent(t *testing.T) {
	for _, reason := range []string{"protected_content", "privacy_unavailable", "frame_unavailable", "privacy_changed"} {
		out := browserControlFailure(&browserautomation.ComponentError{Code: "sensitive_snapshot", Reason: reason}, false)
		if out.Error.Code != "sensitive_snapshot" || out.Error.Reason != reason {
			t.Fatalf("screenshot reason %s lost: %+v", reason, out.Error)
		}
	}
}
