package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/app"
)

func TestAfterGatesDeliveryAndStop(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	carol := h.register("carol", nil)
	parent := h.send(alice, "bob", "first")
	bad := map[string]any{"to": "bob", "body": "bad", "after": parent}
	h.do("POST", "/v1/messages", carol, bad).wantErr(t, 400, "invalid_argument")
	bad["to"] = "carol"
	h.do("POST", "/v1/messages", alice, bad).wantErr(t, 400, "invalid_argument")
	child := h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "second", "after": parent}).want(t, 201)
	childID := str(child.body, "message", "id")
	if str(child.body, "message", "after") != parent {
		t.Fatalf("after missing from response: %s", child.raw)
	}
	if got := h.do("GET", "/v1/overview", alice, nil).want(t, 200); strings.Contains(string(got.raw), "\"queued\":2") {
		t.Fatalf("blocked child counted as queued: %s", got.raw)
	}
	if got := messages(t, h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)); len(got) != 1 || str(got[0], "id") != parent {
		t.Fatalf("blocked child pulled: %v", got)
	}
	// A queued child can be withdrawn by its sender before its parent closes.
	h.do("POST", "/v1/messages/"+childID+"/stop", alice, nil).want(t, 200)
	if got := messages(t, h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)); len(got) != 0 {
		t.Fatalf("withdrawn child pulled: %v", got)
	}
	released := h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "third", "after": parent}).want(t, 201)
	releasedID := str(released.body, "message", "id")
	h.do("POST", "/v1/messages/"+parent+"/reply", bob, map[string]any{"body": "done"}).want(t, 201)
	if got := messages(t, h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)); len(got) != 1 || str(got[0], "id") != releasedID {
		t.Fatalf("released child missing: %v", got)
	}
	late := h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "late", "after": parent}).want(t, 201)
	lateID := str(late.body, "message", "id")
	if got := messages(t, h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)); len(got) != 1 || str(got[0], "id") != lateID {
		t.Fatalf("message after closed task should deliver immediately: %v", got)
	}
}

func TestSteerAttachmentsAndConsume(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	carol := h.register("carol", nil)
	id := h.send(alice, "bob", "work")
	req, _ := http.NewRequest("POST", h.url+"/v1/files?name=note.txt", strings.NewReader("correction file"))
	req.Header.Set("Authorization", "Bearer "+alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.NewDecoder(res.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("upload: %d", res.StatusCode)
	}
	fid := file["id"].(string)
	h.do("GET", "/v1/files/"+fid+"/meta", bob, nil).wantErr(t, 404, "not_found")
	steer := h.do("POST", "/v1/messages/"+id+"/steer", alice, map[string]any{"body": "use this", "attachments": []string{fid}}).want(t, 201)
	if got := steer.body["message"].(map[string]any)["attachments"].([]any); len(got) != 1 {
		t.Fatalf("steer attachments: %s", steer.raw)
	}
	h.do("GET", "/v1/files/"+fid+"/meta", bob, nil).want(t, 200)
	h.do("GET", "/v1/messages/"+id+"/steers", carol, nil).wantErr(t, 403, "permission_denied")
	read := h.do("GET", "/v1/messages/"+id+"/steers?consume=false", bob, nil).want(t, 200)
	if got := steerMessages(t, read); len(got) != 1 || str(got[0], "body") != "use this" {
		t.Fatalf("unread steers: %s", read.raw)
	}
	read = h.do("GET", "/v1/messages/"+id+"/steers?consume=true", bob, nil).want(t, 200)
	if len(steerMessages(t, read)) != 1 {
		t.Fatalf("consume: %s", read.raw)
	}
	read = h.do("GET", "/v1/messages/"+id+"/steers", bob, nil).want(t, 200)
	if len(steerMessages(t, read)) != 0 {
		t.Fatalf("consume did not clear: %s", read.raw)
	}
}

func TestConsoleAfterAndSteerAttachment(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	bob := h.register("bob", nil)
	parent := h.do("POST", "/v1/console/messages", adminToken, map[string]any{"to": "bob", "body": "first"}).want(t, 201)
	id := str(parent.body, "message", "id")
	child := h.do("POST", "/v1/console/messages", adminToken, map[string]any{"to": "bob", "body": "next", "after": id}).want(t, 201)
	childID := str(child.body, "message", "id")
	if str(child.body, "message", "after") != id {
		t.Fatalf("console after lost: %s", child.raw)
	}
	got := messages(t, h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200))
	if len(got) != 1 || str(got[0], "id") != id {
		t.Fatalf("console child pulled early: %v", got)
	}
	req, _ := http.NewRequest("POST", h.url+"/v1/console/files?name=fix.txt", strings.NewReader("fix"))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.NewDecoder(res.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("console upload: %d", res.StatusCode)
	}
	fid := file["id"].(string)
	steer := h.do("POST", "/v1/console/messages/"+id+"/steer", adminToken, map[string]any{"body": "correction", "attachments": []string{fid}}).want(t, 201)
	if len(steer.body["attachments"].([]any)) != 1 {
		t.Fatalf("console steer attachment missing: %s", steer.raw)
	}
	h.do("GET", "/v1/files/"+fid+"/meta", bob, nil).want(t, 200)
	h.do("POST", "/v1/messages/"+id+"/reply", bob, map[string]any{"body": "done"}).want(t, 201)
	got = messages(t, h.do("POST", "/v1/inbox/pull", bob, map[string]any{"types": []string{"task"}}).want(t, 200))
	if len(got) != 1 || str(got[0], "id") != childID {
		t.Fatalf("console child not released: %v", got)
	}
}

func steerMessages(t *testing.T, r resp) []map[string]any {
	t.Helper()
	list, ok := r.body["steers"].([]any)
	if !ok {
		t.Fatalf("no steers array: %s", r.raw)
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		out = append(out, item.(map[string]any))
	}
	return out
}
