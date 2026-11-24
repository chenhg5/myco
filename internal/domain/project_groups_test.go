package domain

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
)

func groupFixture() (*Project, []*ProjectMember) {
	p := &Project{Name: "myco", Groups: []string{"前端", "后端"}}
	ms := []*ProjectMember{
		{Agent: "web", Group: "前端"},
		{Agent: "api", Group: "后端"},
		{Agent: "db", Group: "后端"},
		{Agent: "lead"},
	}
	return p, ms
}

func groupsOf(ms []*ProjectMember) map[AgentName]string {
	out := map[AgentName]string{}
	for _, m := range ms {
		out[m.Agent] = m.Group
	}
	return out
}

func TestApplyGroupsRenameAndMove(t *testing.T) {
	p, ms := groupFixture()
	now := time.Unix(100, 0)
	// Rename 后端 to 服务端: the new list, and its members moved over.
	groups := []string{" 前端 ", "服务端", "应用"}
	changed, err := p.ApplyGroups(ms, &groups, map[AgentName]string{"api": "服务端", "db": "服务端", "lead": "应用"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Groups, []string{"前端", "服务端", "应用"}) || !p.UpdatedAt.Equal(now) {
		t.Fatalf("groups %q at %v", p.Groups, p.UpdatedAt)
	}
	want := map[AgentName]string{"web": "前端", "api": "服务端", "db": "服务端", "lead": "应用"}
	if got := groupsOf(ms); !maps.Equal(got, want) || len(changed) != 3 {
		t.Fatalf("members %v (changed %d)", got, len(changed))
	}
}

func TestApplyGroupsDeleteUngroupsMembers(t *testing.T) {
	p, ms := groupFixture()
	groups := []string{"前端"}
	changed, err := p.ApplyGroups(ms, &groups, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := groupsOf(ms); got["api"] != "" || got["db"] != "" || got["web"] != "前端" || len(changed) != 2 {
		t.Fatalf("members %v (changed %d)", got, len(changed))
	}
	// Assigning alone keeps the list; "" takes a member out.
	changed, err = p.ApplyGroups(ms, nil, map[AgentName]string{"web": "", "lead": "前端"}, time.Now())
	if err != nil || len(changed) != 2 || !slices.Equal(p.Groups, []string{"前端"}) {
		t.Fatalf("assign: %v %d %q", err, len(changed), p.Groups)
	}
	empty := []string{}
	if _, err := p.ApplyGroups(ms, &empty, nil, time.Now()); err != nil || p.Groups == nil || len(p.Groups) != 0 {
		t.Fatalf("no groups: %v %#v", err, p.Groups)
	}
}

func TestApplyGroupsRejects(t *testing.T) {
	long := strings.Repeat("长", MaxGroupName+1)
	many := make([]string, MaxProjectGroups+1)
	for i := range many {
		many[i] = "g" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	for name, c := range map[string]struct {
		groups *[]string
		assign map[AgentName]string
		want   string
	}{
		"unknown member": {nil, map[AgentName]string{"ghost": "前端"}, "ghost is not a member"},
		"unknown group":  {nil, map[AgentName]string{"web": "设计"}, `no group "设计"`},
		"deleted group":  {&[]string{"前端"}, map[AgentName]string{"api": "后端"}, `no group "后端"`},
		"empty name":     {&[]string{"前端", "  "}, nil, "invalid group name"},
		"too long":       {&[]string{long}, nil, "invalid group name"},
		"control":        {&[]string{"a\nb"}, nil, "invalid group name"},
		"duplicate":      {&[]string{"前端", " 前端"}, nil, "listed twice"},
		"too many":       {&many, nil, "at most 30 groups"},
	} {
		t.Run(name, func(t *testing.T) {
			p, ms := groupFixture()
			_, err := p.ApplyGroups(ms, c.groups, c.assign, time.Now())
			if !IsCode(err, CodeInvalidArgument) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want invalid_argument %q, got %v", c.want, err)
			}
			// Nothing changed.
			if !slices.Equal(p.Groups, []string{"前端", "后端"}) || groupsOf(ms)["api"] != "后端" {
				t.Fatalf("changed on error: %q %v", p.Groups, groupsOf(ms))
			}
		})
	}
	ok := make([]string, MaxProjectGroups)
	copy(ok, many)
	if _, err := ParseProjectGroups(ok); err != nil {
		t.Fatalf("30 groups: %v", err)
	}
	if _, err := ParseGroupName(strings.Repeat("长", MaxGroupName)); err != nil {
		t.Fatalf("40 characters: %v", err)
	}
}

func TestCheckMemberGroup(t *testing.T) {
	if g, err := CheckMemberGroup([]string{"前端"}, " 前端 "); err != nil || g != "前端" {
		t.Fatalf("%q %v", g, err)
	}
	if g, err := CheckMemberGroup(nil, ""); err != nil || g != "" {
		t.Fatalf("ungrouped: %q %v", g, err)
	}
	if _, err := CheckMemberGroup([]string{"前端"}, "后端"); !IsCode(err, CodeInvalidArgument) {
		t.Fatalf("unknown: %v", err)
	}
}
