package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
)

func TestFilterTemplates(t *testing.T) {
	ts := []api.Template{
		{ID: "tp_myco_ledger", Slug: "myco/ledger", Name: "记账本", Description: "记收入和支出", Category: "life"},
		{ID: "tp_myco_todo", Slug: "myco/todo", Name: "待办清单", Description: "一起记的待办", Category: "tools"},
		{ID: "tp_ann_weekly", Slug: "ann/weekly", Name: "Weekly", Description: "Our weekly report"}, // no category: other
	}
	names := func(ts []api.Template) string {
		var out []string
		for _, t := range ts {
			out = append(out, t.Slug)
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct{ category, query, want string }{
		{"", "", "myco/ledger,myco/todo,ann/weekly"},
		{"life", "", "myco/ledger"},
		{"LIFE ", "", "myco/ledger"},
		{"other", "", "ann/weekly"},
		{"", "记", "myco/ledger,myco/todo"},
		{"", "记 支出", "myco/ledger"},
		{"", "WEEKLY report", "ann/weekly"},
		{"tools", "支出", ""},
	} {
		if got := names(filterTemplates(ts, c.category, c.query)); got != c.want {
			t.Errorf("filter(%q, %q) = %q, want %q", c.category, c.query, got, c.want)
		}
	}
}

// The list an agent reads stays short: no icons, data structures, guides
// or versions (myco template get has them).
func TestBriefTemplatesLeaveOutTheBulk(t *testing.T) {
	ts := []api.Template{{
		ID: "tp_myco_slides", Slug: "myco/slides", Name: "幻灯片", Description: "排版好的幻灯片", Kind: "custom",
		Icon: strings.Repeat("<path d='M0 0'/>", 200), Guide: strings.Repeat("guide ", 300),
		Data:     map[string]api.TemplateCollection{"slides": {Fields: map[string]string{"title": "text"}}},
		Versions: []api.TemplateVersion{{Version: "1.0.0"}}, Version: "1.0.0", Publisher: "myco", Official: true,
	}}
	b, err := json.Marshal(briefTemplates(ts))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, gone := range []string{"\"icon\"", "\"guide\"", "\"versions\"", "\"data\"", "<path"} {
		if strings.Contains(s, gone) {
			t.Errorf("brief list has %s: %s", gone, s)
		}
	}
	for _, kept := range []string{"myco/slides", "幻灯片", "排版好的幻灯片", "\"category\":\"other\"", "template get"} {
		if !strings.Contains(s, kept) {
			t.Errorf("brief list lacks %s: %s", kept, s)
		}
	}
	if len(s) > 400 {
		t.Errorf("one template's brief entry is %d bytes", len(s))
	}
}
