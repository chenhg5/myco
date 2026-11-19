package domain

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// An Automation sends a task, as its owner, to one agent when its schedule
// comes round or its webhook is called. The hub runs it (each workspace's
// own), so it fires while the owner's computers are off.
type Automation struct {
	ID   string
	Name string
	// Owner is the member who made it ("" on hubs without members); From is
	// the address its tasks are sent from ("@console:<name>").
	Owner   string
	OwnerID string
	From    AgentName
	To      AgentName
	Project string
	// Body is the message; "{{payload}}" is replaced by a webhook's body.
	Body string
	// Cron and TZ: the schedule ("" = none).
	Cron string
	TZ   string
	// WebhookHash is the hash of the webhook's secret ("" = no webhook).
	WebhookHash string
	SkipIfBusy  bool
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// NextRunAt is the next scheduled run (zero: none).
	NextRunAt time.Time
	// LastTask is the id of the last task it sent.
	LastTask string
}

// Run triggers.
const (
	TriggerSchedule = "schedule"
	TriggerWebhook  = "webhook"
	TriggerManual   = "manual"
)

// Run outcomes.
const (
	RunSent    = "sent"
	RunSkipped = "skipped"
	RunFailed  = "failed"
)

// AutomationRun is one time an automation ran.
type AutomationRun struct {
	ID           string
	AutomationID string
	At           time.Time
	Trigger      string
	Outcome      string
	MessageID    string
	Error        string
	// Request is what a webhook call carried (webhook runs only).
	Request *WebhookRequest
}

// WebhookRequest is what a webhook call carried, kept with its run so its
// owner can see what came in: the body (at most 16 KiB) and the headers
// that are safe to show (never credentials, cookies or addresses).
type WebhookRequest struct {
	ContentType string
	Headers     map[string]string
	Body        string
	Size        int
}

// Limits.
const (
	MaxAutomationName = 80
	MaxAutomationBody = 8 << 10
	MaxAutomations    = 200
	// MaxAutomationRuns are kept per automation.
	MaxAutomationRuns = 200
	// MinScheduleSpacing is the least time between two runs of a schedule.
	MinScheduleSpacing = 10 * time.Minute
	// PayloadPlaceholder in a body is replaced by a webhook call's body.
	PayloadPlaceholder = "{{payload}}"
)

// AutomationRepository persists automations and their runs.
type AutomationRepository interface {
	Create(ctx context.Context, a *Automation) error
	Save(ctx context.Context, a *Automation) error
	Get(ctx context.Context, id string) (*Automation, error)
	List(ctx context.Context) ([]*Automation, error)
	Count(ctx context.Context) (int, error)
	Delete(ctx context.Context, id string) error
	// Due lists enabled automations whose next run is at or before now.
	Due(ctx context.Context, now time.Time) ([]*Automation, error)
	AddRun(ctx context.Context, r *AutomationRun, keep int) error
	// Runs lists an automation's runs, newest first.
	Runs(ctx context.Context, automationID string, limit int) ([]*AutomationRun, error)
	// Run is one of an automation's runs.
	Run(ctx context.Context, automationID, id string) (*AutomationRun, error)
	// RunsSince counts runs of one trigger since t (rate limits).
	RunsSince(ctx context.Context, automationID, trigger string, since time.Time) (int, error)
}

// ValidateAutomation checks what people set.
func (a *Automation) Validate() error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return Errf(CodeInvalidArgument, "an automation needs a name")
	}
	if utf8.RuneCountInString(a.Name) > MaxAutomationName {
		return Errf(CodeInvalidArgument, "automation name is longer than %d characters", MaxAutomationName)
	}
	if strings.TrimSpace(a.Body) == "" {
		return Errf(CodeInvalidArgument, "an automation needs the message it sends (body)")
	}
	if len(a.Body) > MaxAutomationBody {
		return Errf(CodeInvalidArgument, "automation body is larger than %d bytes", MaxAutomationBody)
	}
	if a.To == "" {
		return Errf(CodeInvalidArgument, "an automation needs the agent it sends to (to)")
	}
	if a.Cron == "" && a.WebhookHash == "" {
		return Errf(CodeInvalidArgument, "an automation needs a schedule or a webhook (or both)").
			WithSuggestion("set a cron schedule (e.g. \"0 9 * * 1-5\") or turn the webhook on")
	}
	if a.Cron != "" {
		s, err := ParseSchedule(a.Cron, a.TZ)
		if err != nil {
			return err
		}
		if err := s.CheckSpacing(); err != nil {
			return err
		}
	}
	return nil
}

// WithPayload is the message for a run: a webhook's body replaces
// {{payload}}, or, when the message has none, follows it.
func WithPayload(body, payload string) string {
	if strings.Contains(body, PayloadPlaceholder) {
		return strings.ReplaceAll(body, PayloadPlaceholder, payload)
	}
	if strings.TrimSpace(payload) == "" {
		return body
	}
	return strings.TrimRight(body, "\n") + "\n\n---\n" + payload
}

// Schedule is a parsed 5-field cron expression in a time zone.
type Schedule struct {
	Expr string
	Loc  *time.Location
	min  [60]bool
	hour [24]bool
	dom  [32]bool
	mon  [13]bool
	dow  [7]bool
	// domStar / dowStar: the field was "*" (cron's rule: when both day
	// fields are restricted, either matches).
	domStar, dowStar bool
}

// ParseSchedule parses "minute hour day-of-month month day-of-week"
// (numbers, *, a-b, a,b, */n, a-b/n; day of week 0-7, 0 and 7 Sunday) in
// the IANA zone tz ("" = UTC).
func ParseSchedule(expr, tz string) (*Schedule, error) {
	loc := time.UTC
	if tz != "" {
		l, err := time.LoadLocation(tz)
		if err != nil {
			return nil, Errf(CodeInvalidArgument, "unknown time zone %q", tz).WithSuggestion("use an IANA zone such as Asia/Shanghai or America/New_York")
		}
		loc = l
	}
	f := strings.Fields(expr)
	if len(f) != 5 {
		return nil, Errf(CodeInvalidArgument, "a schedule has 5 fields (minute hour day-of-month month day-of-week), got %q", expr).
			WithSuggestion("e.g. \"0 9 * * 1-5\" for 9:00 on weekdays")
	}
	s := &Schedule{Expr: strings.Join(f, " "), Loc: loc}
	if err := field(f[0], 0, 59, s.min[:]); err != nil {
		return nil, err
	}
	if err := field(f[1], 0, 23, s.hour[:]); err != nil {
		return nil, err
	}
	if err := field(f[2], 1, 31, s.dom[:]); err != nil {
		return nil, err
	}
	if err := field(f[3], 1, 12, s.mon[:]); err != nil {
		return nil, err
	}
	var dow [8]bool
	if err := field(f[4], 0, 7, dow[:]); err != nil {
		return nil, err
	}
	for i := 0; i < 7; i++ {
		s.dow[i] = dow[i]
	}
	if dow[7] {
		s.dow[0] = true
	}
	s.domStar, s.dowStar = f[2] == "*", f[4] == "*"
	return s, nil
}

func field(f string, lo, hi int, set []bool) error {
	bad := func() error {
		return Errf(CodeInvalidArgument, "bad schedule field %q (allowed %d-%d, *, a-b, a,b, */n)", f, lo, hi)
	}
	for _, part := range strings.Split(f, ",") {
		if part == "" {
			return bad()
		}
		step := 1
		if i := strings.IndexByte(part, '/'); i >= 0 {
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n < 1 {
				return bad()
			}
			step, part = n, part[:i]
		}
		a, b := lo, hi
		switch {
		case part == "*":
		case strings.Contains(part, "-"):
			x, y, _ := strings.Cut(part, "-")
			var e1, e2 error
			a, e1 = strconv.Atoi(x)
			b, e2 = strconv.Atoi(y)
			if e1 != nil || e2 != nil || a > b {
				return bad()
			}
		default:
			n, err := strconv.Atoi(part)
			if err != nil {
				return bad()
			}
			a, b = n, n
			if step > 1 {
				b = hi // "5/15": from 5 on
			}
		}
		if a < lo || b > hi {
			return bad()
		}
		for v := a; v <= b; v += step {
			set[v] = true
		}
	}
	return nil
}

func (s *Schedule) dayMatches(t time.Time) bool {
	d, w := s.dom[t.Day()], s.dow[int(t.Weekday())]
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return w
	case s.dowStar:
		return d
	default:
		return d || w
	}
}

// Next is the first time after t (to the minute) the schedule fires, in
// its zone; zero if none within five years.
func (s *Schedule) Next(t time.Time) time.Time {
	prev := t.In(s.Loc)
	lt := prev.Truncate(time.Minute).Add(time.Minute)
	end := lt.AddDate(5, 0, 0)
	for lt.Before(end) {
		if !s.mon[int(lt.Month())] {
			lt = time.Date(lt.Year(), lt.Month()+1, 1, 0, 0, 0, 0, s.Loc)
			continue
		}
		if !s.dayMatches(lt) {
			lt = time.Date(lt.Year(), lt.Month(), lt.Day()+1, 0, 0, 0, 0, s.Loc)
			continue
		}
		if !s.hour[lt.Hour()] {
			next := time.Date(lt.Year(), lt.Month(), lt.Day(), lt.Hour()+1, 0, 0, 0, s.Loc)
			if !next.After(lt) { // a clock turned back: step past it
				next = lt.Add(time.Hour).Truncate(time.Hour)
			}
			lt = next
			continue
		}
		if !s.min[lt.Minute()] {
			lt = lt.Add(time.Minute)
			continue
		}
		// A clock turned back shows the same minute twice: once a day.
		if sameWallMinute(lt, prev) {
			lt = lt.Add(time.Minute)
			continue
		}
		return lt.UTC()
	}
	return time.Time{}
}

// CheckSpacing refuses a schedule that fires twice within
// MinScheduleSpacing (looking at the next two years of firings).
func (s *Schedule) CheckSpacing() error {
	// Within one day the minute and hour fields decide it: check the
	// minutes of the day the schedule can fire at, wrapping to the next day.
	var mins []int
	for h := 0; h < 24; h++ {
		if !s.hour[h] {
			continue
		}
		for m := 0; m < 60; m++ {
			if s.min[m] {
				mins = append(mins, h*60+m)
			}
		}
	}
	gap := 24 * 60
	for i := 1; i < len(mins); i++ {
		gap = min(gap, mins[i]-mins[i-1])
	}
	if len(mins) > 0 {
		gap = min(gap, mins[0]+24*60-mins[len(mins)-1]) // last of a day to the next day's first
	}
	if time.Duration(gap)*time.Minute < MinScheduleSpacing {
		return Errf(CodeInvalidArgument, "schedule %q fires more often than every %d minutes", s.Expr, int(MinScheduleSpacing.Minutes())).
			WithSuggestion("space runs at least %d minutes apart, e.g. \"*/15 * * * *\" or \"0 * * * *\"", int(MinScheduleSpacing.Minutes()))
	}
	return nil
}

func sameWallMinute(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay() && a.Hour() == b.Hour() && a.Minute() == b.Minute()
}
