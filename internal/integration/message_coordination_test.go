package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/cli"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/domain"
)

func TestMessageTreeAndReassignThroughHubAndCLI(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	profiles := map[string]*cliEnv{}
	clients := map[string]*client.Client{}
	for _, name := range []string{"boss", "lead", "old", "new", "coordinator", "outsider"} {
		path := filepath.Join(home, name+".yaml")
		profiles[name] = newCLI(t, h, path)
		profiles[name].must(c, "agent", "register", "--name", name)
		// Register returns the token once; read only this test's profile.
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		clients[name] = h.client(cfg.Token)
	}
	if _, err := h.admin().CreateProject(c, api.CreateProjectRequest{Name: "acme"}); err != nil {
		t.Fatal(err)
	}
	role := "coordinator"
	for name := range profiles {
		in := api.ProjectMemberRequest{}
		if name == "coordinator" {
			in.Role = &role
		}
		if _, err := h.admin().PutProjectMember(c, "acme", name, in); err != nil {
			t.Fatal(err)
		}
	}
	var root api.SendResponse
	if err := json.Unmarshal([]byte(profiles["boss"].must(c, "message", "send", "--to", "lead", "--body", "plan work", "--project", "acme")), &root); err != nil {
		t.Fatal(err)
	}
	if _, err := clients["lead"].Pull(c, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := clients["lead"].Ack(c, root.Message.ID); err != nil {
		t.Fatal(err)
	}
	profiles["lead"].env["MYCO_MESSAGE_ID"] = root.Message.ID
	profiles["lead"].env["MYCO_THREAD_ID"] = root.Message.ThreadID
	file := filepath.Join(t.TempDir(), "demo.txt")
	if err := os.WriteFile(file, []byte("fixture attachment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const body = "keep this original body unchanged"
	var child api.SendResponse
	if err := json.Unmarshal([]byte(profiles["lead"].must(c, "message", "send", "--to", "old", "--body", body, "--attach", file)), &child); err != nil {
		t.Fatal(err)
	}
	if child.Message.ParentID != root.Message.ID || child.Message.Project != "acme" || len(child.Message.Attachments) != 1 {
		t.Fatalf("provenance/payload: %+v", child.Message)
	}
	if _, err := clients["lead"].Hold(c, root.Message.ID, child.Message.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tree", "reassign"} {
		schema := profiles["coordinator"].must(c, "schema", "message", name)
		if !strings.Contains(schema, `"id"`) {
			t.Fatalf("missing schema: %s", schema)
		}
		if name == "reassign" && (!strings.Contains(schema, `"dry-run"`) || !strings.Contains(schema, `"note"`)) {
			t.Fatalf("reassign schema: %s", schema)
		}
	}
	var tree api.TaskTreeResponse
	raw := profiles["coordinator"].must(c, "message", "tree", "--id", root.Message.ID)
	if err := json.Unmarshal([]byte(raw), &tree); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, body) || len(tree.Root.Children) != 1 || tree.Root.Children[0].ID != child.Message.ID || len(tree.Root.WaitingOn) != 1 || tree.Root.WaitingOn[0].ID != child.Message.ID {
		t.Fatalf("task metadata tree: %s", raw)
	}
	text := profiles["coordinator"].must(c, "message", "tree", "--id", root.Message.ID, "--format", "text")
	if !strings.Contains(text, "└─ ") || !strings.Contains(text, "waits:task="+child.Message.ID) {
		t.Fatalf("text tree: %s", text)
	}
	if _, err := clients["outsider"].ReassignMessage(c, child.Message.ID, api.ReassignRequest{To: "new"}, false); err == nil {
		t.Fatal("ordinary project member reassigned another sender's task")
	}
	if _, err := clients["outsider"].MessageTree(c, root.Message.ID, 100); err == nil {
		t.Fatal("ordinary project member read an unrelated task tree")
	}
	code, _, stderr := profiles["coordinator"].run(c, "message", "reassign", "--id", child.Message.ID, "--to", "new", "--note", "extra context", "--dry-run")
	if code != cli.ExitDryRun {
		t.Fatalf("dry run: %d %s", code, stderr)
	}
	unchanged, err := clients["lead"].GetMessage(c, child.Message.ID)
	if err != nil || unchanged.To != "old" || unchanged.ReassignNote != "" {
		t.Fatalf("preview mutated task: %+v %v", unchanged, err)
	}
	audits, err := h.admin().Audit(c, client.AuditFilter{Target: child.Message.ID})
	if err != nil || len(audits) != 0 {
		t.Fatalf("preview wrote audit: %+v %v", audits, err)
	}
	var moved api.ReassignResponse
	if err := json.Unmarshal([]byte(profiles["coordinator"].must(c, "message", "reassign", "--id", child.Message.ID, "--to", "new", "--note", "extra context")), &moved); err != nil {
		t.Fatal(err)
	}
	if moved.ID != child.Message.ID || moved.PreviousTo != "old" || moved.To != "new" || moved.Project != "acme" || moved.ThreadID != child.Message.ThreadID {
		t.Fatalf("reassignment result: %+v", moved)
	}
	if got, err := clients["old"].Pull(c, 10, 0); err != nil || len(got) != 0 {
		t.Fatalf("old recipient got moved task: %+v %v", got, err)
	}
	if _, err := clients["old"].GetMessage(c, child.Message.ID); err == nil {
		t.Fatal("old recipient still reads moved task")
	}
	got, err := clients["new"].Pull(c, 10, 0)
	if err != nil || len(got) != 1 || got[0].ID != child.Message.ID || got[0].Body != body || got[0].From != "lead" || got[0].ReassignNote != "extra context" {
		t.Fatalf("new recipient payload: %+v %v", got, err)
	}
	_, dest, err := clients["new"].DownloadFile(c, got[0].Attachments[0].FileID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "fixture attachment\n" {
		t.Fatalf("attachment: %q %v", data, err)
	}
	audits, err = h.admin().Audit(c, client.AuditFilter{Target: child.Message.ID})
	if err != nil || len(audits) != 1 || audits[0].Action != "message.reassign" || audits[0].Actor != "coordinator" || strings.Contains(audits[0].Detail, "extra context") {
		t.Fatalf("audit: %+v %v", audits, err)
	}
	if _, err := clients["coordinator"].ReassignMessage(c, child.Message.ID, api.ReassignRequest{To: "old"}, false); err == nil {
		t.Fatal("delivered task moved")
	}
	if err := clients["new"].Ack(c, child.Message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := clients["coordinator"].ReassignMessage(c, child.Message.ID, api.ReassignRequest{To: "old"}, false); err == nil {
		t.Fatal("acked task moved")
	}
	if _, err := clients["new"].Reply(c, child.Message.ID, "completed", false); err != nil {
		t.Fatal(err)
	}
	if err := h.h.Watches.Tick(c); err != nil {
		t.Fatal(err)
	}
	resumed, err := clients["lead"].GetMessage(c, root.Message.ID)
	if err != nil || resumed.Status != "queued" {
		t.Fatalf("hold reference lost by reassignment: %+v %v", resumed, err)
	}
	if _, err := clients["coordinator"].ReassignMessage(c, root.Message.ID, api.ReassignRequest{To: "new"}, false); err == nil {
		t.Fatal("previously acked task became reassignable after hold wake")
	}
	sealed, err := clients["lead"].Send(c, api.SendRequest{To: "old", Body: domain.SealedPrefix + "synthetic-envelope", Project: "acme"}, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = clients["lead"].ReassignMessage(c, sealed.Message.ID, api.ReassignRequest{To: "new"}, false)
	if ce, ok := err.(*client.Error); !ok || ce.Code != "unsupported" || !strings.Contains(ce.Suggestion, "stop + send") {
		t.Fatalf("E2E rejection: %v", err)
	}
	// Knowing an ID must not let a coordinator forward a hidden private task
	// to a visible worker and obtain its body. The sender still owns its body.
	privateTask, err := clients["lead"].Send(c, api.SendRequest{To: "old", Body: "private work", Project: "acme"}, false)
	if err != nil {
		t.Fatal(err)
	}
	privateInvite, err := h.admin().InviteMember(c, api.InviteRequest{Name: "private-owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.admin().SetAgentOwner(c, "old", "private-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.admin().SetVisibility(c, "old", "private"); err != nil {
		t.Fatal(err)
	}
	if _, err := clients["coordinator"].MessageTree(c, privateTask.Message.ID, 100); err == nil {
		t.Fatal("coordinator read hidden private task metadata")
	}
	if _, err := clients["coordinator"].ReassignMessage(c, privateTask.Message.ID, api.ReassignRequest{To: "new"}, false); err == nil {
		t.Fatal("coordinator forwarded hidden private task")
	}
	if _, err := clients["new"].GetMessage(c, privateTask.Message.ID); err == nil {
		t.Fatal("hidden task body became visible through failed reassignment")
	}
	if _, err := clients["lead"].ReassignMessage(c, privateTask.Message.ID, api.ReassignRequest{To: "new"}, false); err != nil {
		t.Fatalf("original sender lost reassignment permission: %v", err)
	}

	// Hub management does not let an original console sender use somebody
	// else's private computer. The original owner's own nodes remain valid.
	consoleTask, err := h.admin().ConsoleSend(c, api.SendRequest{To: "new", Body: "console work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.admin().ReassignMessage(c, consoleTask.Message.ID, api.ReassignRequest{To: "old"}, false); err == nil {
		t.Fatal("admin bypassed original console sender's private-recipient boundary")
	}
	paired, err := h.client("").Pair(c, privateInvite.Link[strings.Index(privateInvite.Link, "/c/")+3:], "private-device")
	if err != nil {
		t.Fatal(err)
	}
	ownerClient := h.client(paired.Token)
	ownerTask, err := ownerClient.ConsoleSend(c, api.SendRequest{To: "new", Body: "owner work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.admin().ReassignMessage(c, ownerTask.Message.ID, api.ReassignRequest{To: "old"}, false); err != nil {
		t.Fatalf("original console owner could not use own private node: %v", err)
	}

}
