package app

import (
	"testing"

	"github.com/myco-ai/myco/internal/domain"
)

// Who may change a page's records: everyone who sees it, unless the page
// (or a collection of its template) is for those who manage it.
func TestPageEditors(t *testing.T) {
	s := &PageService{}
	pub := Actor{Agent: &domain.Agent{Name: "writer", Owner: "m_ann"}}  // published it
	mate := Actor{Agent: &domain.Agent{Name: "helper", Owner: "m_ann"}} // an agent of the same person
	other := Actor{Agent: &domain.Agent{Name: "eve", Owner: "m_bob"}}   // sees it through its project
	owner := Actor{Member: &domain.Member{ID: "m_ann"}}
	manifest := `{"id":"ann/board","name":"看板","version":"1.0.0","data":{"cards":{"fields":{"title":"text"}},"settings":{"single":true,"write":"managers","fields":{"theme":"text"}}}}`
	p := &domain.Page{ID: "pg_1", Publisher: "writer", Owner: "m_ann", Project: "team", Manifest: manifest}
	for _, c := range []struct {
		who        Actor
		coll       string
		pageOnly   bool
		want, also bool
	}{
		{pub, "cards", false, true, true},
		{mate, "settings", false, true, true},
		{owner, "settings", false, true, true},
		{other, "cards", false, true, false},     // everyone may; with the page for managers, not
		{other, "settings", false, false, false}, // the template keeps its settings for managers
		{other, "", false, true, false},          // the shared state
	} {
		p.Editors = domain.PageEditorsEveryone
		if got := s.CanEdit(c.who, p, c.coll); got != c.want {
			t.Errorf("%+v %q (everyone): %v", c.who.Agent, c.coll, got)
		}
		p.Editors = domain.PageEditorsManagers
		if got := s.CanEdit(c.who, p, c.coll); got != c.also {
			t.Errorf("%+v %q (managers): %v", c.who.Agent, c.coll, got)
		}
	}
	if _, err := domain.ParsePageEditors("friends"); err == nil {
		t.Fatal("only everyone or managers")
	}
	if e, err := domain.ParsePageEditors("everyone"); err != nil || e != "" {
		t.Fatal("everyone is the default (empty)")
	}
	if _, err := domain.ParseManifest([]byte(`{"id":"ann/x","name":"x","version":"1.0.0","data":{"a":{"write":"admins","fields":{"t":"text"}}}}`)); err == nil {
		t.Fatal("a collection's write is managers or nothing")
	}
}
