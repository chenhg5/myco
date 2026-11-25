package templates

import (
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"testing"
)

func TestRetiredPollCannotReturnFromArchive(t *testing.T) {
	item := func(id, version string) app.OfficialTemplate {
		return app.OfficialTemplate{Manifest: &domain.Manifest{ID: id, Version: version}}
	}
	list, newer := merge([]app.OfficialTemplate{item("myco/poll", "1.0.0"), item("myco/notes", "1.0.0")}, []app.OfficialTemplate{item("myco/poll", "99.0.0"), item("myco/notes", "2.0.0")})
	if len(list) != 1 || list[0].Manifest.ID != "myco/notes" || len(newer) != 1 || newer[0] != "myco/notes" {
		t.Fatal("retired app returned", list, newer)
	}
}
