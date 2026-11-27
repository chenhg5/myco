package api

import "time"

// TimingSummary uses seconds. Missing samples are null, never zero.
type TimingSummary struct {
	Samples       int      `json:"samples"`
	MedianSeconds *float64 `json:"median_seconds"`
	P90Seconds    *float64 `json:"p90_seconds"`
}
type AgentTaskStats struct {
	Name            string        `json:"name"`
	Tasks           int           `json:"tasks"`
	Done            int           `json:"done"`
	Failed          int           `json:"failed"`
	Stopped         int           `json:"stopped"`
	SuccessRate     *float64      `json:"success_rate"`
	Redone          int           `json:"redone"`
	RedoneShare     *float64      `json:"redone_share"`
	ReceiptToReply  TimingSummary `json:"receipt_to_reply"`
	QueueWait       TimingSummary `json:"queue_wait"`
	CommonTaskTypes []string      `json:"common_task_types"`
}
type AgentStatsList struct {
	Version int              `json:"version"`
	Since   time.Time        `json:"since"`
	Until   time.Time        `json:"until"`
	Scope   string           `json:"scope"`
	Agents  []AgentTaskStats `json:"agents"`
}
type AgentPick struct {
	Name            string         `json:"name"`
	Score           float64        `json:"score"`
	Available       bool           `json:"available"`
	Online          bool           `json:"online"`
	Queued          int            `json:"queued"`
	InProgress      int            `json:"in_progress"`
	RunnerSlotsFree *int           `json:"runner_slots_free"`
	Quota           string         `json:"quota"`
	Reason          string         `json:"reason"`
	Stats           AgentTaskStats `json:"stats"`
}
type AgentPickList struct {
	Version     int         `json:"version"`
	GeneratedAt time.Time   `json:"generated_at"`
	Scope       string      `json:"scope"`
	Candidates  []AgentPick `json:"candidates"`
}
type CoordinatorStats struct {
	Version              int           `json:"version"`
	Name                 string        `json:"name"`
	Since                time.Time     `json:"since"`
	Until                time.Time     `json:"until"`
	Scope                string        `json:"scope"`
	PersonTasks          int           `json:"person_tasks"`
	ReceiptToReply       TimingSummary `json:"receipt_to_reply"`
	DelegatedTasks       int           `json:"delegated_tasks"`
	QueueSamples         int           `json:"queue_samples"`
	DelegatedQueuedShare *float64      `json:"delegated_queued_share"`
	Reassigned           int           `json:"reassigned"`
	Withdrawn            int           `json:"withdrawn"`
	PersonFollowups      int           `json:"person_followups"`
	ReassignmentMethod   string        `json:"reassignment_method"`
}
