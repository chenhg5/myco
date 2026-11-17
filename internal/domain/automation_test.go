package domain

import (
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // zones on systems without a database (Windows)
)

func mustSchedule(t *testing.T, expr, tz string) *Schedule {
	t.Helper()
	s, err := ParseSchedule(expr, tz)
	if err != nil {
		t.Fatalf("%q: %v", expr, err)
	}
	return s
}

func TestScheduleNext(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct{ expr, tz, from, want string }{
		{"0 9 * * *", "", "2026-11-16T08:59:30Z", "2026-11-16T09:00:00Z"},
		{"0 9 * * *", "", "2026-11-16T09:00:00Z", "2026-11-17T09:00:00Z"},
		// weekdays only: Friday evening goes to Monday
		{"0 9 * * 1-5", "", "2026-11-20T10:00:00Z", "2026-11-23T09:00:00Z"},
		// a zone: 9:00 in Shanghai is 01:00 UTC
		{"0 9 * * *", "Asia/Shanghai", "2026-11-16T02:00:00Z", "2026-11-17T01:00:00Z"},
		{"*/15 * * * *", "", "2026-11-16T10:07:00Z", "2026-11-16T10:15:00Z"},
		{"5,35 */2 * * *", "", "2026-11-16T10:40:00Z", "2026-11-16T12:05:00Z"},
		// day of week 7 is Sunday too
		{"0 8 * * 7", "", "2026-11-16T00:00:00Z", "2026-11-22T08:00:00Z"},
		// day of month and day of week both set: either matches
		{"0 0 1 * 1", "", "2026-11-25T00:00:00Z", "2026-11-30T00:00:00Z"},
		{"0 12 1 1 *", "", "2026-11-16T00:00:00Z", "2027-01-01T12:00:00Z"},
		{"0 0 29 2 *", "", "2026-03-01T00:00:00Z", "2028-02-29T00:00:00Z"},
		// spring forward in New York: 2:30 does not exist on 2026-03-08, so
		// that day has no run; the next is the day after (06:30 UTC, EDT)
		{"30 2 * * *", "America/New_York", "2026-03-07T08:00:00Z", "2026-03-09T06:30:00Z"},
	} {
		got := mustSchedule(t, tc.expr, tc.tz).Next(at(tc.from))
		if !got.Equal(at(tc.want)) {
			t.Errorf("%q in %q after %s: %s, want %s", tc.expr, tc.tz, tc.from, got.Format(time.RFC3339), tc.want)
		}
	}
	// Fall back in New York (2026-11-01): 1:30 comes twice; it runs once.
	s := mustSchedule(t, "30 1 * * *", "America/New_York")
	first := s.Next(at("2026-11-01T04:00:00Z"))
	if !first.Equal(at("2026-11-01T05:30:00Z")) {
		t.Fatalf("first 1:30: %s", first)
	}
	if second := s.Next(first); !second.Equal(at("2026-11-02T06:30:00Z")) {
		t.Fatalf("1:30 ran twice on the day the clocks turned back: next %s", second)
	}
}

func TestScheduleParseErrors(t *testing.T) {
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8", "*/0 * * * *", "5-1 * * * *", "a * * * *", "1,,2 * * * *"} {
		if _, err := ParseSchedule(bad, ""); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
	if _, err := ParseSchedule("0 9 * * *", "Mars/Olympus"); err == nil || !strings.Contains(err.Error(), "time zone") {
		t.Fatalf("unknown zone: %v", err)
	}
}

func TestScheduleSpacing(t *testing.T) {
	for _, ok := range []string{"*/10 * * * *", "0 * * * *", "0,30 9 * * *", "58 23,0 * * *", "0 9 * * 1-5"} {
		if err := mustSchedule(t, ok, "").CheckSpacing(); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"* * * * *", "*/5 * * * *", "0,5 9 * * *", "55,59 23 * * *"} {
		if err := mustSchedule(t, bad, "").CheckSpacing(); err == nil {
			t.Errorf("%q fires more often than every 10 minutes", bad)
		}
	}
}

func TestWithPayload(t *testing.T) {
	if got := WithPayload("构建失败：{{payload}}", "exit 1"); got != "构建失败：exit 1" {
		t.Fatal(got)
	}
	if got := WithPayload("看一下", "{\"a\":1}"); got != "看一下\n\n---\n{\"a\":1}" {
		t.Fatal(got)
	}
	if got := WithPayload("看一下", "  "); got != "看一下" {
		t.Fatal(got)
	}
}

func TestAutomationValidate(t *testing.T) {
	a := &Automation{Name: " 日报 ", To: "pm", Body: "写日报", Cron: "0 9 * * *"}
	if err := a.Validate(); err != nil || a.Name != "日报" {
		t.Fatal(err, a.Name)
	}
	for _, b := range []*Automation{
		{Name: "", To: "pm", Body: "x", Cron: "0 9 * * *"},
		{Name: "x", To: "pm", Body: " ", Cron: "0 9 * * *"},
		{Name: "x", To: "", Body: "x", Cron: "0 9 * * *"},
		{Name: "x", To: "pm", Body: "x"}, // neither schedule nor webhook
		{Name: "x", To: "pm", Body: "x", Cron: "* * * * *"},
		{Name: strings.Repeat("名", 81), To: "pm", Body: "x", WebhookHash: "h"},
		{Name: "x", To: "pm", Body: strings.Repeat("x", MaxAutomationBody+1), WebhookHash: "h"},
	} {
		if err := b.Validate(); err == nil {
			t.Errorf("%+v should not validate", b)
		}
	}
}
