package cli

import "testing"

func TestBridgeName(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"feishu", "coder"}:                           "feishu-coder",
		{"slack", "PM (Codex)"}:                       "slack-pm-codex",
		{"lark", "cloud_hdk.01"}:                      "lark-cloud-hdk-01",
		{"feishu", "a-very-long-agent-name-x"}:        "feishu-a-very-long-agent-name-x",
		{"feishu", "a-very-long-agent-name-xyz-more"}: "feishu-a-very-long-agent-name-xy",
	} {
		if got := BridgeName(in[0], in[1]); got != want {
			t.Errorf("%v: %q, want %q", in, got, want)
		}
	}
}
