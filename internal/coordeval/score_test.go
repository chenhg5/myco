package coordeval

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestAnAnswerCountsOnlyAsAWholeResult(t *testing.T) {
	list := Item{Name: "amber", Numbers: []int{50, 7}}
	token := Item{Name: "harbor-map", Token: "MAP-7Q2"}
	for text, want := range map[string]bool{
		"amber: total 57": true, "amber: 57.": true, "amber: 157": false, "amber: 570": false,
	} {
		if answers(text, list) != want {
			t.Errorf("answers(%q) for 57 = %v", text, !want)
		}
	}
	if !answers("harbor-map: done, result MAP-7Q2", token) || answers("MAP-7Q2X", token) {
		t.Error("a token is a whole word")
	}
}

func TestTwoAgentsEditingOneFileAtOnceIsACollision(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	ev := func(s int, agent, kind string) Event {
		return Event{At: at(s), Agent: agent, Kind: kind, Item: agent + "-item", Detail: "settings.toml"}
	}
	apart := &Run{End: at(100), Events: []Event{ev(0, "a", EvEditStart), ev(5, "a", EvEditEnd), ev(6, "b", EvEditStart), ev(9, "b", EvEditEnd)}}
	if _, _, clash := apart.overlappingEdits(); clash {
		t.Fatal("one after the other is no collision")
	}
	together := &Run{End: at(100), Events: []Event{ev(0, "a", EvEditStart), ev(3, "b", EvEditStart), ev(5, "a", EvEditEnd), ev(9, "b", EvEditEnd)}}
	if a, b, clash := together.overlappingEdits(); !clash || a != "a" || b != "b" {
		t.Fatalf("at once: %v %v %v", a, b, clash)
	}
	// an edit that never ended (the agent went away) lasts to the end
	gone := &Run{End: at(100), Events: []Event{ev(0, "a", EvEditStart), ev(50, "b", EvEditStart), ev(55, "b", EvEditEnd)}}
	if _, _, clash := gone.overlappingEdits(); !clash {
		t.Fatal("an unfinished edit still holds the file")
	}
}

// Scores: wrong is 0, a broken constraint is 0, and the rest deduct.
func TestScoring(t *testing.T) {
	w := &World{opts: Options{Unit: time.Second}, catalog: map[string]Item{"amber": {Name: "amber"}, "birch": {Name: "birch"}}}
	t0 := time.Unix(1_800_000_000, 0)
	msg := func(s int, from, to string, typ domain.MessageType, body string, ask bool) *domain.Message {
		return &domain.Message{From: domain.AgentName(from), To: domain.AgentName(to), Type: typ, Body: body, Ask: ask, CreatedAt: t0.Add(time.Duration(s) * time.Second)}
	}
	sc := &Scenario{Name: "t", Title: "T", Ideal: 4, check: func(r *Run, res *Result) { res.Elapsed = 10 }}
	r := &Run{Scenario: sc, World: w, fakes: map[string]bool{"worker": true}, Messages: []*domain.Message{
		msg(1, Lead, "worker", domain.TypeTask, "- amber: 1, 2", false),
		msg(2, Lead, "worker", domain.TypeTask, "- amber: 1, 2 (again)", false),
		msg(3, Lead, User, domain.TypeProgress, "Which colour do you like?", true),
		msg(4, Lead, User, domain.TypeProgress, "Halfway there.", false),
	}}
	res := score(r)
	if !res.Passed || res.Interruptions != 1 || res.Waste != 1 || res.Ratio != 2.5 {
		t.Fatalf("%+v", res)
	}
	// 100 - 10 (a question) - 5 (one again) - 10 (2.5x: (2.5-1.5)*10)
	if res.Score != 75 {
		t.Fatalf("score %d: %v", res.Score, res.Deductions)
	}
	sc.check = func(r *Run, res *Result) { res.fail("no answer") }
	if res := score(r); res.Passed || res.Score != 0 || !strings.HasPrefix(res.Deductions[0], "result: no answer") {
		t.Fatalf("a wrong result: %+v", res)
	}
	sc.check = func(r *Run, res *Result) { res.veto("left out the constraint") }
	if res := score(r); res.Passed || res.Score != 0 || res.Veto == "" {
		t.Fatalf("a veto: %+v", res)
	}
}

func TestTheReportIsAMycoUITable(t *testing.T) {
	rep := &Report{Coordinator: "baseline", Passed: 1, Total: 2, Mean: 50, Results: []Result{
		{Title: "Five in parallel", Passed: true, Score: 100, Ratio: 1.2},
		{Title: "A wrong result", Score: 0, Deductions: []string{"result: the wrong total reached the person"}},
	}}
	table := rep.Table()
	start := strings.Index(table, "```myco-ui\n")
	end := strings.LastIndex(table, "\n```")
	if start < 0 || end < start {
		t.Fatalf("no myco-ui block:\n%s", table)
	}
	var block struct {
		Type  string `json:"type"`
		Props struct {
			Columns []string   `json:"columns"`
			Rows    [][]string `json:"rows"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(table[start+len("```myco-ui\n"):end]), &block); err != nil {
		t.Fatal(err)
	}
	if block.Type != "Table" || len(block.Props.Rows) != 2 || len(block.Props.Rows[0]) != len(block.Props.Columns) || block.Props.Rows[1][6] != "result: the wrong total reached the person" {
		t.Fatalf("%+v", block)
	}
}
