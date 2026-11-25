package console

import (
	"testing"

	"github.com/myco-ai/myco/internal/service"
)

type fakeHubService struct {
	st      service.Status
	started int
}

func (f *fakeHubService) Name() string                                   { return "fake" }
func (f *fakeHubService) PlanInstall(service.Spec) (service.Plan, error) { return service.Plan{}, nil }
func (f *fakeHubService) PlanUninstall() service.Plan                    { return service.Plan{} }
func (f *fakeHubService) Status() (service.Status, error)                { return f.st, nil }
func (f *fakeHubService) Restart() error                                 { return nil }
func (f *fakeHubService) Start() error                                   { f.started++; f.st.Running = true; return nil }

// A workspace service that is installed but stopped starts again, at most
// once a minute; one that is running, or was taken away (停止 removes it),
// is left alone.
func TestStartStoppedHub(t *testing.T) {
	f := &fakeHubService{st: service.Status{Installed: true}}
	was := serviceFor
	serviceFor = func(string, service.ID) (service.Manager, error) { return f, nil }
	defer func() { serviceFor = was }()

	l := &Local{Home: t.TempDir()}
	if !l.startStoppedHub("") || f.started != 1 {
		t.Fatalf("stopped hub not started: %d", f.started)
	}
	f.st.Running = false
	if l.startStoppedHub("") || f.started != 1 {
		t.Fatal("started again within a minute")
	}
	l2 := &Local{Home: t.TempDir()}
	f.st = service.Status{Installed: false}
	if l2.startStoppedHub("") {
		t.Fatal("a hub stopped with 停止 (no service) must stay stopped")
	}
	l3 := &Local{Home: t.TempDir(), NoServiceManager: true}
	f.st = service.Status{Installed: true}
	if l3.startStoppedHub("") {
		t.Fatal("no service manager: the App runs its hubs itself")
	}
}
