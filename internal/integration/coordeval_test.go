package integration_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/coordeval"
	"github.com/myco-ai/myco/internal/testutil"
)

// fakeCoordinator stands in for Claude Code as the coordinator under test:
// it hands the whole goal to the first agent through the real myco CLI and
// answers with what came back. A coordinator this naive gets the right
// result but nothing in parallel; what matters here is the way there.
const fakeCoordinator = `#!/bin/sh
case " $* " in *" --version "*) echo "2.1.300 (Claude Code)"; exit 0;; esac
prompt=$(cat)
myco agent list --format json >/dev/null || exit 1
answer=$(printf '%s' "$prompt" | myco message send --to counter-a --type task --project drill --body-file - --wait 60s --format json |
  python3 -c 'import json,sys; d=json.load(sys.stdin); r=d.get("reply") or {}; print(r.get("body",""))')
python3 - "$answer" <<'PY'
import json, sys
print(json.dumps({"type": "system", "subtype": "init", "session_id": "coord-sess-1"}))
print(json.dumps({"type": "result", "subtype": "success", "is_error": False, "result": sys.argv[1], "session_id": "coord-sess-1"}))
PY
`

// The coordination test drives a real agent CLI the way `make coord-eval`
// does: myco's runner serves the coordinator with it, the session's myco
// talks to the drill's hub as the coordinator, and the run is scored from
// the hub's records.
func TestTheCoordinationTestDrivesARealCLIThroughTheRunner(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("needs python3 for the fake CLI")
	}
	home := isolatedHome(t)
	bin := t.TempDir()
	myco := filepath.Join(bin, "myco")
	build := exec.Command("go", "build", "-o", myco, "../../cmd/myco")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building myco: %v\n%s", err, out)
	}
	claude := filepath.Join(bin, "fake-claude")
	if err := os.WriteFile(claude, []byte(fakeCoordinator), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = home
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var log bytes.Buffer
	res, run, err := coordeval.RunScenario(ctx, coordeval.ScenarioByName("parallel-five"),
		coordeval.CLI{Harness: "claude", Myco: myco, Binary: claude, Log: &log},
		coordeval.RunOptions{Options: coordeval.Options{Dir: t.TempDir(), Unit: 300 * time.Millisecond, Seed: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Correct {
		t.Fatalf("the CLI's answer: %+v\nrunner log:\n%s", res, log.String())
	}
	parallel := false
	for _, d := range res.Deductions {
		parallel = parallel || strings.Contains(d, "nothing ran in parallel")
	}
	if !parallel || res.Score > 70 {
		t.Fatalf("one agent did all five, and it did not count: %+v", res)
	}
	// it went the way a real session would: tasks from the coordinator
	// through the hub, to the agent, answered to the person
	sent := 0
	for _, m := range run.Messages {
		if string(m.From) == coordeval.Lead && string(m.To) == "counter-a" {
			sent++
		}
	}
	if sent != 1 {
		t.Fatalf("tasks from the coordinator: %d", sent)
	}
}
