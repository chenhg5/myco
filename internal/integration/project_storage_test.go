package integration_test

import (
	"bytes"
	"fmt"
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/testutil"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectStorageAPI(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	p, e := admin.CreateProject(c, api.CreateProjectRequest{Name: "storage"})
	if e != nil {
		t.Fatal(e)
	}
	if p.FilesMax != 1000 || p.BytesMax != 2<<30 || p.DocsMax != 100 || !p.CanManageStorage {
		t.Fatalf("defaults %+v", p.ProjectStorage)
	}
	if _, e = admin.PutProjectDoc(c, "storage", "guide", "abc"); e != nil {
		t.Fatal(e)
	}
	data := []byte{0, 1, 2, 3}
	if _, e = admin.PutProjectFile(c, "storage", "demo.png", "image/png", bytes.NewReader(data), int64(len(data))); e != nil {
		t.Fatal(e)
	}
	q, e := admin.ProjectStorage(c, "storage")
	if e != nil || q.FilesUsed != 1 || q.DocsUsed != 1 || q.BytesUsed != 7 {
		t.Fatalf("usage %+v %v", q, e)
	}
	if _, e = admin.SetProjectStorage(c, "storage", api.ProjectStorageRequest{FilesMax: 1, BytesMax: 7}); e != nil {
		t.Fatal(e)
	}
	if _, e = admin.PutProjectFile(c, "storage", "more.png", "image/png", bytes.NewReader(data), 4); e == nil {
		t.Fatal("quota was not enforced by upload endpoint")
	}
	p, e = admin.GetProject(c, "storage", true)
	if e != nil || p.FilesUsed != 1 || p.BytesMax != 7 || p.Docs[0].Name == "" {
		t.Fatalf("project usage %+v %v", p, e)
	}
	if _, e = admin.SetProjectStorage(c, "storage", api.ProjectStorageRequest{}); e != nil {
		t.Fatal(e)
	}
	q, e = admin.ProjectStorage(c, "storage")
	if e != nil || q.FilesMax != 1000 || q.BytesMax != 2<<30 || q.BytesUsed != 7 {
		t.Fatalf("restore defaults %+v %v", q, e)
	}
	// A project owner with an ordinary workspace account still cannot grant storage.
	inv, e := admin.InviteMember(c, api.InviteRequest{Name: "member"})
	if e != nil {
		t.Fatal(e)
	}
	pair, e := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "member-device")
	if e != nil {
		t.Fatal(e)
	}
	member := h.client(pair.Token)
	own, e := member.CreateProject(c, api.CreateProjectRequest{Name: "owned"})
	if e != nil {
		t.Fatal(e)
	}
	if own.CanManageStorage {
		t.Fatal("ordinary project owner gained storage management")
	}
	if _, e = member.SetProjectStorage(c, "owned", api.ProjectStorageRequest{FilesMax: 2000, BytesMax: 4 << 30}); e == nil {
		t.Fatal("ordinary owner raised storage")
	}
	if _, e = admin.SetProjectStorage(c, "owned", api.ProjectStorageRequest{FilesMax: 10001, BytesMax: 4 << 30}); e == nil {
		t.Fatal("server accepted limits over protection cap")
	}
}

func TestProjectStorageHubPolicyAndDiskGuard(t *testing.T) {
	for _, lowDisk := range []bool{false, true} {
		t.Run(fmt.Sprint(lowDisk), func(t *testing.T) {
			policy := app.DefaultProjectStoragePolicy()
			policy.FilesMax = 2
			policy.BytesMax = 4
			policy.DiskMinFreeBytes = 0
			policy.DiskMinFreePercent = 0
			if lowDisk {
				policy.DiskMinFreeBytes = 1 << 62
			}
			native, e := hubserver.Build(hubserver.Options{DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: adminToken, Policy: app.DefaultMessagePolicy(), OfficialDir: testutil.OfficialDir(t), ProjectStorage: &policy})
			if e != nil {
				t.Fatal(e)
			}
			srv := httptest.NewServer(native.Handler)
			h := &hub{t: t, h: native, url: srv.URL, srv: srv}
			t.Cleanup(h.stop)
			c := ctx(t)
			admin := h.admin()
			if _, e = admin.CreateProject(c, api.CreateProjectRequest{Name: "configured"}); e != nil {
				t.Fatal(e)
			}
			q, e := admin.ProjectStorage(c, "configured")
			if e != nil || q.FilesMax != 2 || q.BytesMax != 4 {
				t.Fatalf("configured %+v %v", q, e)
			}
			data := []byte{0, 1, 2, 3}
			_, e = admin.PutProjectFile(c, "configured", "x.png", "image/png", bytes.NewReader(data), 4)
			if lowDisk {
				if e == nil || !strings.Contains(e.Error(), "disk space") {
					t.Fatalf("disk guard: %v", e)
				}
			} else if e != nil {
				t.Fatal(e)
			}
			if lowDisk {
				q, e = admin.ProjectStorage(c, "configured")
				if e != nil || q.FilesUsed != 0 || q.BytesUsed != 0 {
					t.Fatalf("rejected write left metadata: %+v %v", q, e)
				}
			}
		})
	}
}
