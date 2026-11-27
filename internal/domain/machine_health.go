package domain

import (
	"math"
	"strings"
	"time"
)

// MachineHealth is a bounded, argument-free observation made by a runner.
// Complete=false means unknown, never a fresh healthy observation.
type MachineHealth struct {
	OwnerID         string           `json:"owner_id,omitempty"`
	Incident        string           `json:"incident,omitempty"`
	At              time.Time        `json:"at"`
	RunnerInstance  string           `json:"runner_instance"`
	Complete        bool             `json:"complete"`
	Alert           bool             `json:"alert"`
	LoadRatio       float64          `json:"load_ratio"`
	LoadMetric      string           `json:"load_metric"` // load1_per_core | cpu_busy
	Cores           int              `json:"cores"`
	MemoryTotal     uint64           `json:"memory_total"`
	MemoryAvailable uint64           `json:"memory_available"`
	Processes       []MachineProcess `json:"processes"`
}

// MachineProcess deliberately has no command line, environment or executable path.
type MachineProcess struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	Created    string  `json:"created"` // opaque OS creation identity; not a PID alone
	CPU        float64 `json:"cpu"`     // percent of one core
	Memory     uint64  `json:"memory"`
	Agent      string  `json:"agent,omitempty"`
	Suspicious bool    `json:"suspicious"`
	Ref        string  `json:"ref,omitempty"` // short-lived runner reference, only for verified suspects
}

// Validate bounds all caller-controlled strings and numbers before persistence.
func (h MachineHealth) Validate() error {
	if len(h.OwnerID) > 128 || strings.ContainsAny(h.OwnerID, "\x00\r\n\t") || len(h.Incident) > 120 || strings.ContainsAny(h.Incident, "\x00\r\n") || h.At.IsZero() || len(h.RunnerInstance) > 120 || strings.ContainsAny(h.RunnerInstance, "\x00\r\n\t") || len(h.Processes) > 20 || h.Cores < 0 || h.Cores > 65536 || h.MemoryAvailable > h.MemoryTotal || h.LoadRatio < 0 || math.IsNaN(h.LoadRatio) || math.IsInf(h.LoadRatio, 0) {
		return Errf(CodeInvalidArgument, "invalid machine health sample")
	}
	if h.Complete && (h.Cores < 1 || h.MemoryTotal == 0 || h.LoadMetric == "") {
		return Errf(CodeInvalidArgument, "complete health sample lacks measurements")
	}
	if h.LoadMetric != "" && h.LoadMetric != "load1_per_core" && h.LoadMetric != "cpu_busy" {
		return Errf(CodeInvalidArgument, "invalid machine load metric")
	}
	for _, p := range h.Processes {
		if p.Name == "" || p.Created == "" || p.PID <= 1 || p.CPU < 0 || math.IsNaN(p.CPU) || math.IsInf(p.CPU, 0) || len(p.Name) > 100 || len(p.Created) > 128 || len(p.Agent) > 120 || len(p.Ref) > 128 {
			return Errf(CodeInvalidArgument, "invalid machine process")
		}
		for _, s := range []string{p.Name, p.Created, p.Agent, p.Ref} {
			if strings.ContainsAny(s, "\x00\r\n\t") {
				return Errf(CodeInvalidArgument, "invalid machine process label")
			}
		}
		if p.Ref != "" && (!p.Suspicious || p.Agent == "" || !h.Complete) {
			return Errf(CodeInvalidArgument, "unverified process reference")
		}
	}
	return nil
}
