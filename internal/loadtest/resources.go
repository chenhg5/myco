package loadtest

import "time"

type processResource struct {
	CPUTicks                         uint64   `json:"cpu_ticks"`
	CPUPercent                       *float64 `json:"cpu_pct,omitempty"`
	RSSBytes                         int64    `json:"rss_bytes"`
	Threads, FDs                     int
	LogicalRead, DiskRead, DiskWrite uint64
}
type resourceSample struct {
	At             time.Time
	Supported      bool
	Hub, Generator processResource
	DiskFree       *uint64       `json:"disk_free_bytes,omitempty"`
	DiskTotal      *uint64       `json:"disk_total_bytes,omitempty"`
	CPU            []uint64      `json:"system_cpu_ticks,omitempty"`
	StealPercent   *float64      `json:"steal_pct,omitempty"`
	IOWaitPercent  *float64      `json:"iowait_pct,omitempty"`
	Disk           *diskResource `json:"disk,omitempty"`
}
type diskResource struct {
	Device                                             string
	Writes, WriteMS, Flushes, FlushMS, BusyMS, QueueMS uint64
	WriteAwaitMS, FlushAwaitMS, UtilPercent            *float64
}
