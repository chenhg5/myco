package domain

import (
	"reflect"
	"testing"
)

func TestAllowFromEdits(t *testing.T) {
	list := func(s ...string) *[]string { return &s }
	for _, tt := range []struct {
		name                      string
		edit                      AllowFromEdit
		want                      []AgentName
		added, removed, unchanged []string
		bad                       bool
	}{
		{"add", AllowFromEdit{Add: list("new", "new", "boss")}, []AgentName{"boss", "console", "new"}, []string{"new"}, []string{}, []string{"boss", "console"}, false},
		{"remove", AllowFromEdit{Remove: list("boss", "missing")}, []AgentName{"console"}, []string{}, []string{"boss"}, []string{"console"}, false},
		{"replace", AllowFromEdit{Replace: list("new")}, []AgentName{"new"}, []string{"new"}, []string{"boss", "console"}, []string{}, false},
		{"both", AllowFromEdit{Add: list("new"), Remove: list("boss")}, []AgentName{"console", "new"}, []string{"new"}, []string{"boss"}, []string{"console"}, false},
		{"mixed modes", AllowFromEdit{Replace: list("new"), Add: list("new")}, nil, nil, nil, nil, true},
		{"overlap", AllowFromEdit{Add: list("boss"), Remove: list("boss")}, nil, nil, nil, nil, true},
		{"last sender", AllowFromEdit{Remove: list("boss", "console")}, nil, nil, nil, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{AllowFrom: []AgentName{"boss", "console"}, Binding: Binding{Harness: HarnessExec}}
			d, err := tt.edit.Apply(a, false)
			if tt.bad {
				if err == nil {
					t.Fatal("expected rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a.AllowFrom, tt.want) || !reflect.DeepEqual(d.Added, tt.added) || !reflect.DeepEqual(d.Removed, tt.removed) || !reflect.DeepEqual(d.Unchanged, tt.unchanged) {
				t.Fatalf("got %v, %#v", a.AllowFrom, d)
			}
		})
	}
}

func TestAllowFromUnrestrictedDelta(t *testing.T) {
	for _, before := range [][]AgentName{nil, {AnySender}} {
		a := &Agent{AllowFrom: before}
		add, remove := []string{"new"}, []string{"old"}
		if _, err := (AllowFromEdit{Add: &add}).Apply(a, false); err != nil || !reflect.DeepEqual(a.AllowFrom, before) {
			t.Fatalf("append must not narrow anyone: %v", err)
		}
		if _, err := (AllowFromEdit{Remove: &remove}).Apply(a, false); err == nil {
			t.Fatal("cannot exclude one sender from anyone")
		}
	}
}
