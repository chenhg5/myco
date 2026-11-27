package cli

import (
	"github.com/myco-ai/myco/internal/api"
	"testing"
)

func TestProcessStopChecksMachineAndAgentOwnership(t *testing.T) {
	owner, other := "owner", "other"
	expected := &api.Agent{OwnerID: owner, MachineID: "machine"}
	for _, tc := range []struct {
		name    string
		current *api.Agent
		want    bool
	}{
		{"current", &api.Agent{OwnerID: owner, MachineID: "machine", MachineOwnerID: &owner}, true},
		{"machine transferred alone", &api.Agent{OwnerID: owner, MachineID: "machine", MachineOwnerID: &other}, false},
		{"node transferred", &api.Agent{OwnerID: other, MachineID: "machine", MachineOwnerID: &owner}, false},
		{"different machine", &api.Agent{OwnerID: owner, MachineID: "replacement", MachineOwnerID: &owner}, false},
		{"older Hub", &api.Agent{OwnerID: owner, MachineID: "machine"}, false},
		{"unreadable", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := processOwnerCurrent(tc.current, expected); got != tc.want {
				t.Fatalf("allowed=%v, want %v", got, tc.want)
			}
		})
	}
}
