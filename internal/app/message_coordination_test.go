package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func TestMessageTreeParentsThreadsAndWaits(t *testing.T) {
	c := context.Background()
	f := setup(t)
	boss, lead := f.agent(t, "boss"), f.agent(t, "lead")
	worker := f.agent(t, "worker")
	f.agent(t, "helper")
	send := func(a *domain.Agent, in app.SendInput) *domain.Message {
		t.Helper()
		out, err := f.messages.Send(c, a, in)
		if err != nil {
			t.Fatal(err)
		}
		return out.Message
	}
	root := send(boss, app.SendInput{To: "lead", Body: "root"})
	child := send(lead, app.SendInput{To: "worker", Body: "child", ParentID: root.ID, ThreadID: root.ThreadID})
	grand := send(worker, app.SendInput{To: "helper", Body: "grandchild", ParentID: child.ID, ThreadID: root.ThreadID})
	legacy := send(lead, app.SendInput{To: "helper", Body: "old runner child", ThreadID: root.ThreadID})
	if _, err := f.messages.Pull(c, lead, 1, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.Transition(c, lead, root.ID, app.TransitionAck, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.HoldUntil(c, lead, root.ID, child.ID, false); err != nil {
		t.Fatal(err)
	}
	out, err := f.messages.MessageTree(c, app.Actor{Agent: boss}, root.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if out.Root.Message.ID != root.ID || len(out.Root.Waits) != 1 || out.Root.Waits[0].ID != child.ID || len(out.Root.Children) != 2 {
		t.Fatalf("root: %+v", out.Root)
	}
	var explicit, inferred *app.TaskTree
	for _, n := range out.Root.Children {
		if n.Message.ID == child.ID {
			explicit = n
		}
		if n.Message.ID == legacy.ID {
			inferred = n
		}
	}
	if explicit == nil || explicit.Relation != "parent" || len(explicit.Children) != 1 || explicit.Children[0].Message.ID != grand.ID {
		t.Fatalf("explicit tree: %+v", explicit)
	}
	if inferred == nil || inferred.Relation != "thread_inferred" {
		t.Fatalf("legacy tree: %+v", inferred)
	}
	if out.Root.Message.Body != "" || explicit.Message.Body != "" {
		t.Fatal("tree fetched message contents")
	}
	if _, err := f.messages.MessageTree(c, app.Actor{Agent: f.agent(t, "stranger")}, root.ID, 100); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("unrelated root visible: %v", err)
	}
	limited, err := f.messages.MessageTree(c, app.Actor{Agent: boss}, root.ID, 1)
	if err != nil || !limited.Truncated || len(limited.Root.Children) != 0 {
		t.Fatalf("limit: %+v %v", limited, err)
	}
	if _, err := f.messages.Send(c, boss, app.SendInput{To: "helper", Body: "forged delegation", ParentID: root.ID}); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("forged parent: %v", err)
	}
}

func TestMessageTreePrunesPrivateDescendants(t *testing.T) {
	c := context.Background()
	f := setup(t)
	boss, lead := f.agent(t, "boss"), f.agent(t, "lead")
	f.agent(t, "private-helper")
	if _, err := f.agents.SetOwner(c, "private-helper", "another-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.agents.SetVisibility(c, "private-helper", "private"); err != nil {
		t.Fatal(err)
	}
	root, err := f.messages.Send(c, boss, app.SendInput{To: "lead", Body: "root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.messages.Send(c, lead, app.SendInput{To: "private-helper", Body: "private", ParentID: root.Message.ID})
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.messages.MessageTree(c, app.Actor{Agent: boss}, root.Message.ID, 100)
	if err != nil || len(out.Root.Children) != 0 {
		t.Fatalf("private descendant leaked: %+v %v", out, err)
	}
	if _, err := f.messages.MessageTree(c, app.Actor{Agent: boss}, child.Message.ID, 100); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("private root visible: %v", err)
	}
}

func TestReassignPermissionsDryRunAndDelivery(t *testing.T) {
	c := context.Background()
	f := setup(t)
	boss, old, newAgent, outsider := f.agent(t, "boss"), f.agent(t, "old"), f.agent(t, "new"), f.agent(t, "outsider")
	sent, err := f.messages.Send(c, boss, app.SendInput{To: string(old.Name), Body: "original body"})
	if err != nil {
		t.Fatal(err)
	}
	id := sent.Message.ID
	if _, err := f.messages.Reassign(c, app.Actor{Agent: outsider}, id, string(newAgent.Name), "", false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("outsider: %v", err)
	}
	preview, err := f.messages.Reassign(c, app.Actor{Agent: boss}, id, string(newAgent.Name), "context", true)
	if err != nil || !preview.DryRun || preview.Message.To != newAgent.Name {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	unchanged, err := f.messages.Get(c, boss, id)
	if err != nil || unchanged.To != old.Name || unchanged.ReassignNote != "" {
		t.Fatalf("dry run changed storage: %+v %v", unchanged, err)
	}
	moved, err := f.messages.Reassign(c, app.Actor{Agent: boss}, id, string(newAgent.Name), "context", false)
	if err != nil || moved.Message.ID != id || moved.Message.Body != "original body" || moved.Message.From != boss.Name || moved.Message.ThreadID != sent.Message.ThreadID || moved.Message.ReassignNote != "context" {
		t.Fatalf("transfer: %+v %v", moved, err)
	}
	got, err := f.messages.Pull(c, old, 10, 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("old recipient got task: %+v %v", got, err)
	}
	got, err = f.messages.Pull(c, newAgent, 10, 0)
	if err != nil || len(got) != 1 || got[0].ID != id {
		t.Fatalf("new recipient: %+v %v", got, err)
	}
	if _, err := f.messages.Reassign(c, app.Actor{Agent: boss}, id, string(old.Name), "", false); !domain.IsCode(err, domain.CodeInvalidState) || !strings.Contains(err.Error(), "queued") {
		t.Fatalf("delivered task transfer: %v", err)
	}
	if _, err := f.messages.Transition(c, newAgent, id, app.TransitionAck, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.Reassign(c, app.Actor{Admin: true}, id, string(old.Name), "", false); !domain.IsCode(err, domain.CodeInvalidState) {
		t.Fatalf("acked task transfer: %v", err)
	}
}

func TestReassignRejectsE2EAndRecipientAllowlist(t *testing.T) {
	c := context.Background()
	f := setup(t)
	boss := f.agent(t, "boss")
	f.agent(t, "old")
	f.agent(t, "new")
	f.agent(t, "restricted", "someone-else")
	sent, err := f.messages.Send(c, boss, app.SendInput{To: "old", Body: domain.SealedPrefix + "synthetic-encrypted-envelope"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.messages.Reassign(c, app.Actor{Agent: boss}, sent.Message.ID, "new", "", false)
	if !domain.IsCode(err, domain.CodeUnsupported) {
		t.Fatalf("E2E transfer must be unsupported: %v", err)
	}
	var suggestion string
	if e, ok := err.(*domain.Error); ok {
		suggestion = e.Suggestion
	}
	if !strings.Contains(suggestion, "stop + send") {
		t.Fatalf("missing re-encryption recovery: %s", suggestion)
	}
	plain, err := f.messages.Send(c, boss, app.SendInput{To: "old", Body: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.Reassign(c, app.Actor{Admin: true}, plain.Message.ID, "restricted", "", false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("original sender allowlist bypass: %v", err)
	}
}

func TestMessageTreeIncludesBroadcastTaskParents(t *testing.T) {
	c := context.Background()
	f := setup(t)
	boss, lead := f.agent(t, "boss"), f.agent(t, "lead")
	f.agent(t, "worker")
	root, err := f.messages.Send(c, boss, app.SendInput{To: "lead", Body: "delegate"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.messages.Broadcast(c, lead, app.BroadcastInput{Selector: "name=worker", Body: "parallel work", ParentID: root.Message.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 || out.Messages[0].ParentID != root.Message.ID {
		t.Fatalf("missing broadcast parent: %+v", out)
	}
	tree, err := f.messages.MessageTree(c, app.Actor{Agent: boss}, root.Message.ID, 100)
	if err != nil || len(tree.Root.Children) != 1 || tree.Root.Children[0].Message.ID != out.Messages[0].ID {
		t.Fatalf("missing broadcast child: %+v %v", tree, err)
	}
}

func TestReassignRefusesTaskAlreadyDisclosedByInboxWait(t *testing.T) {
	c := context.Background()
	f := setup(t)
	sender, old := f.agent(t, "sender"), f.agent(t, "old")
	f.agent(t, "new")
	sent, err := f.messages.Send(c, sender, app.SendInput{To: "old", Body: "work"})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := f.messages.WaitPending(c, old, 0)
	if err != nil || len(pending) != 1 || pending[0].Status != domain.StatusQueued {
		t.Fatalf("wait must preserve queue state: %+v %v", pending, err)
	}
	if _, err := f.messages.Reassign(c, app.Actor{Agent: sender}, sent.Message.ID, "new", "", false); !domain.IsCode(err, domain.CodeInvalidState) {
		t.Fatalf("disclosed task moved: %v", err)
	}
}

func TestMessageTreeWaitsForConsoleAnswerAfterQuestionIsStoredDone(t *testing.T) {
	c := context.Background()
	f := setup(t)
	worker := f.agent(t, "worker")
	root, err := f.messages.SendFromConsole(c, "demo", app.SendInput{To: "worker", Body: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.Pull(c, worker, 1, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.Transition(c, worker, root.Message.ID, app.TransitionAck, false); err != nil {
		t.Fatal(err)
	}
	question, err := f.messages.Ask(c, worker, root.Message.ID, "which option?", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if question.Status != domain.StatusDone {
		t.Fatalf("fixture should use a closed console question: %+v", question)
	}
	tree, err := f.messages.MessageTree(c, app.Actor{Agent: worker}, root.Message.ID, 100)
	if err != nil || len(tree.Root.Waits) != 1 || tree.Root.Waits[0].Kind != "answer" || tree.Root.Waits[0].ID != question.ID {
		t.Fatalf("lost answer wait: %+v %v", tree, err)
	}
}

func TestMessageTreeLabelsCrossBranchHoldAsDependency(t *testing.T) {
	c := context.Background()
	f := setup(t)
	boss, lead := f.agent(t, "boss"), f.agent(t, "lead")
	f.agent(t, "helper")
	root, err := f.messages.Send(c, boss, app.SendInput{To: "lead", Body: "first task"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.messages.Send(c, boss, app.SendInput{To: "lead", Body: "second task"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.messages.Send(c, lead, app.SendInput{To: "helper", Body: "dependency", ParentID: other.Message.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.Pull(c, lead, 10, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.Transition(c, lead, root.Message.ID, app.TransitionAck, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.HoldUntil(c, lead, root.Message.ID, child.Message.ID, false); err != nil {
		t.Fatal(err)
	}
	tree, err := f.messages.MessageTree(c, app.Actor{Agent: boss}, root.Message.ID, 100)
	if err != nil || len(tree.Root.Children) != 1 || tree.Root.Children[0].Relation != "hold" || tree.Root.Children[0].Message.ParentID != other.Message.ID {
		t.Fatalf("hold confused with parent: %+v %v", tree, err)
	}
}
