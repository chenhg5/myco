package main

import (
	"syscall"
	"testing"
	"time"
)

func TestBrowserReaperRequiresMatchingStartIdentity(t *testing.T) {
	_, cmd, record, done := groupFixture(t)
	_ = syscall.Kill(-record.PGID, syscall.SIGKILL)
	<-done
	deadline := time.Now().Add(3 * time.Second)
	var current []groupProcess
	for {
		var err error
		current, err = groupProcesses(record.PGID)
		if err != nil {
			t.Fatal(err)
		}
		ready := false
		for _, p := range current {
			if p.Zombie {
				ready = true
			}
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture did not become a reapable child")
		}
		time.Sleep(10 * time.Millisecond)
	}
	forged := record
	forged.Members = append([]groupProcess(nil), record.Members...)
	for i := range forged.Members {
		forged.Members[i].Start = "unrelated-process-generation"
	}
	reapKnownBrowserChildren(forged, current, true)
	if gone, _ := groupGone(cmd.Process.Pid); gone {
		t.Fatal("reaped a child whose identity did not match")
	}
	reapKnownBrowserChildren(record, current, true)
	if gone, err := groupGone(record.PGID); err != nil || !gone {
		t.Fatalf("known children were not reaped: %v %v", gone, err)
	}
}
