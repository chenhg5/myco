package domain

import (
	"strings"
	"testing"
)

func TestJoinFeatures(t *testing.T) {
	if got := JoinFeatures([]string{"changes", "ask_access", "changes", "Bad", "", "a-b", strings.Repeat("x", 41)}); got != "changes ask_access" {
		t.Fatalf("JoinFeatures: %q", got)
	}
	if got := (Runtime{Features: "changes ask_access"}).FeatureList(); len(got) != 2 || got[1] != "ask_access" {
		t.Fatalf("FeatureList: %v", got)
	}
	if (Runtime{}).FeatureList() != nil {
		t.Fatal("no features: want nil")
	}
}
