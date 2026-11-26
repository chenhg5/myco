package httpapi

import (
	"encoding/json"
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"testing"
)

func TestStandbyWireKeepsLegacyOffline(t *testing.T) {
	state, standby := agentStateDTO(app.StateStandby)
	body, err := json.Marshal(api.AgentStatus{State: state, Standby: standby})
	if err != nil {
		t.Fatal(err)
	}
	var legacy struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(body, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.State != "offline" || !standby {
		t.Fatalf("legacy state=%q standby=%v", legacy.State, standby)
	}
	for _, want := range []app.AgentState{app.StateWorking, app.StatePending, app.StateIdle, app.StateOffline} {
		state, standby := agentStateDTO(want)
		if state != string(want) || standby {
			t.Fatalf("unexpected state %q standby=%v", state, standby)
		}
	}
}
