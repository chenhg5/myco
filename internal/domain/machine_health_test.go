package domain

import (
	"math"
	"testing"
	"time"
)

func TestMachineHealthRejectsUnsafeReports(t *testing.T) {
	good := MachineHealth{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RunnerInstance: "instance", Complete: true, Cores: 8, MemoryTotal: 100, MemoryAvailable: 50, LoadMetric: "cpu_busy"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*MachineHealth){func(h *MachineHealth) { h.LoadRatio = math.NaN() }, func(h *MachineHealth) { h.MemoryAvailable = 101 }, func(h *MachineHealth) { h.Processes = []MachineProcess{{PID: 20, Name: "worker\nsecret"}} }, func(h *MachineHealth) { h.Processes = []MachineProcess{{PID: 20, Name: "worker", Ref: "ref"}} }, func(h *MachineHealth) { h.Processes = make([]MachineProcess, 21) }} {
		h := good
		change(&h)
		if h.Validate() == nil {
			t.Fatal("accepted unsafe sample")
		}
	}
}

func TestMachineHealthRejectsExecutablePaths(t *testing.T) {
	for _, name := range []string{"/private/worker", `C:\private\worker.exe`} {
		h := MachineHealth{At: time.Now(), Processes: []MachineProcess{{PID: 20, Name: name, Created: "birth"}}}
		if h.Validate() == nil {
			t.Fatal("accepted executable path")
		}
	}
}
