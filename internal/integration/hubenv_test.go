package integration_test

import (
	"path/filepath"
	"testing"

	"github.com/chenhg5/myco/internal/app"
)

// A saved identity talks to the hub that issued its token, even when
// MYCO_HUB points elsewhere: a claude.ai environment sets MYCO_HUB to the
// default workspace for enrollment, and a session that joined another
// workspace must not send that workspace's token there.
func TestSavedIdentityKeepsItsHubOverMYCO_HUB(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	bot := newCLI(t, h, filepath.Join(home, "bot.yaml"))
	bot.must(c, "agent", "register", "--name", "ws-bot")

	bot.env["MYCO_HUB"] = "http://127.0.0.1:9/w/somewhere-else"
	if code, out, errb := bot.run(c, "inbox", "pull"); code != 0 {
		t.Fatalf("the saved identity should reach its own hub: exit %d\n%s%s", code, out, errb)
	}
}
