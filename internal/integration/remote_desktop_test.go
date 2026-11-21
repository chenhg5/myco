package integration_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/remotedesktop"
)

func TestRemoteSignalingPermissionsAndLifecycle(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	node := h.client(h.register(api.RegisterRequest{Name: "my-mac", Binding: api.Binding{Harness: "machine"}}))
	if e := node.ReportMachine(c, api.MachineReport{Fingerprint: "remote-mac-fingerprint-0123456789", Name: "mac", OS: "darwin"}); e != nil {
		t.Fatal(e)
	}
	ms, e := admin.Machines(c)
	if e != nil || len(ms) != 1 {
		t.Fatal(ms, e)
	}
	machine := ms[0].ID
	inv, e := admin.InviteMember(c, api.InviteRequest{Name: "bob"})
	if e != nil {
		t.Fatal(e)
	}
	code := inv.Link[strings.Index(inv.Link, "/c/")+3:]
	pair, e := h.client("").Pair(c, code, "bob-pc")
	if e != nil {
		t.Fatal(e)
	}
	bob := h.client(pair.Token)
	denied := func(e error) {
		t.Helper()
		var ce *client.Error
		if !errors.As(e, &ce) || ce.Code != "permission_denied" {
			t.Fatalf("expected forbidden, got %v", e)
		}
	}
	denied(node.RemoteRequest(c, "PUT", "machines/"+machine, "", remotedesktop.Device{}, nil))
	denied(bob.RemoteRequest(c, "PUT", "machines/"+machine, "", remotedesktop.Device{}, nil))
	var host struct {
		Key string `json:"host_key"`
	}
	if e = admin.RemoteRequest(c, "PUT", "machines/"+machine, "", remotedesktop.Device{OS: "darwin", Name: "My Mac", Control: true}, &host); e != nil || host.Key == "" {
		t.Fatal(e, host)
	}
	mine, e := admin.RemoteDevices(c)
	if e != nil || len(mine) != 1 {
		t.Fatal(mine, e)
	}
	other, e := bob.RemoteDevices(c)
	if e != nil || len(other) != 0 {
		t.Fatal(other, e)
	}
	offer := map[string]any{"machine": machine, "offer": remotedesktop.Offer{Description: remotedesktop.Description{Type: "offer", SDP: "v=0\r\n"}, PairingCode: "pair-secret", Control: true}}
	denied(bob.RemoteRequest(c, "POST", "sessions", "", offer, nil))
	var session remotedesktop.Session
	if e = admin.RemoteRequest(c, "POST", "sessions", "", offer, &session); e != nil || session.Offer != nil {
		t.Fatal(session, e)
	}
	var pending struct {
		Sessions []remotedesktop.Session `json:"sessions"`
	}
	if e = admin.RemoteRequest(c, "GET", "machines/"+machine+"/requests", "wrong", nil, &pending); e == nil {
		t.Fatal("wrong host key accepted")
	}
	if e = admin.RemoteRequest(c, "GET", "machines/"+machine+"/requests", host.Key, nil, &pending); e != nil || len(pending.Sessions) != 1 || pending.Sessions[0].Offer.PairingCode != "pair-secret" {
		t.Fatal(pending, e)
	}
	denied(bob.RemoteRequest(c, "GET", "sessions/"+session.ID, "", nil, nil))
	answer := map[string]any{"answer": remotedesktop.Description{Type: "answer", SDP: "v=0\r\nanswer"}}
	if e = admin.RemoteRequest(c, "PUT", "machines/"+machine+"/sessions/"+session.ID, host.Key, answer, nil); e != nil {
		t.Fatal(e)
	}
	if e = admin.RemoteRequest(c, "GET", "sessions/"+session.ID, "", nil, &session); e != nil || session.Answer == nil || session.State != "connected" {
		t.Fatal(session, e)
	}
	if e = admin.RemoteRequest(c, "DELETE", "sessions/"+session.ID, "", nil, nil); e != nil {
		t.Fatal(e)
	}
	pending.Sessions = nil
	if e = admin.RemoteRequest(c, "GET", "machines/"+machine+"/requests", host.Key, nil, &pending); e != nil || pending.Sessions[0].State != "closed" || pending.Sessions[0].Offer != nil {
		t.Fatal(pending, e)
	}
	if e = admin.RemoteRequest(c, "DELETE", "machines/"+machine, host.Key, nil, nil); e != nil {
		t.Fatal(e)
	}
	mine, e = admin.RemoteDevices(c)
	if e != nil || len(mine) != 0 {
		t.Fatal(mine, e)
	}
	entries, e := admin.Audit(c, client.AuditFilter{})
	if e != nil {
		t.Fatal(e)
	}
	actions := map[string]bool{}
	for _, a := range entries {
		actions[a.Action] = true
		if strings.Contains(a.Detail, "pair-secret") {
			t.Fatal("pairing code in audit")
		}
	}
	for _, a := range []string{"remote.enable", "remote.connect", "remote.disconnect", "remote.disable"} {
		if !actions[a] {
			t.Fatal("missing audit", a)
		}
	}
}

func TestRemoteAppOnlyHostNeedsNoAgentOrMachine(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	token, e := admin.CreateConsoleToken(c, "my-mac-app")
	if e != nil {
		t.Fatal(e)
	}
	owner := h.client(token.Token)
	who, e := owner.Whoami(c)
	if e != nil || who.Device == nil {
		t.Fatal(who, e)
	}
	id := "app:" + who.Device.ID
	var host struct {
		Key string `json:"host_key"`
	}
	if e = owner.RemoteRequest(c, "PUT", "machines/"+id, "", remotedesktop.Device{Name: "My Mac", OS: "darwin"}, &host); e != nil || host.Key == "" {
		t.Fatal(host, e)
	}
	devices, e := owner.RemoteDevices(c)
	if e != nil || len(devices) != 1 || devices[0].Machine != id {
		t.Fatal(devices, e)
	}
	machines, e := admin.Machines(c)
	if e != nil || len(machines) != 0 {
		t.Fatalf("remote app created machine records: %v %v", machines, e)
	}
	// A different member cannot impersonate the host's existing App device ID.
	inv, e := admin.InviteMember(c, api.InviteRequest{Name: "bob"})
	if e != nil {
		t.Fatal(e)
	}
	paired, e := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "bob-pc")
	if e != nil {
		t.Fatal(e)
	}
	bob := h.client(paired.Token)
	if e = bob.RemoteRequest(c, "PUT", "machines/"+id, "", remotedesktop.Device{}, nil); e == nil {
		t.Fatal("another member impersonated an App host")
	}
	devices, e = bob.RemoteDevices(c)
	if e != nil || len(devices) != 0 {
		t.Fatal(devices, e)
	}
	var pending struct {
		Sessions []remotedesktop.Session `json:"sessions"`
	}
	if e = owner.RemoteRequest(c, "GET", "machines/"+id+"/requests", host.Key, nil, &pending); e != nil {
		t.Fatal(e)
	}
	if e = owner.RemoteRequest(c, "DELETE", "machines/"+id, host.Key, nil, nil); e != nil {
		t.Fatal(e)
	}
}
