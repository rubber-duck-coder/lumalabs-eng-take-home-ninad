package domain

import "time"

type WorkloadType string

const (
	WorkloadTypeTraining  WorkloadType = "training"
	WorkloadTypeInference WorkloadType = "inference"
	WorkloadTypeBatch     WorkloadType = "batch"
)

type WorkloadPriority string

const (
	WorkloadPriorityLow    WorkloadPriority = "low"
	WorkloadPriorityNormal WorkloadPriority = "normal"
	WorkloadPriorityHigh   WorkloadPriority = "high"
)

type WorkloadState string

const (
	WorkloadStatePending   WorkloadState = "pending"
	WorkloadStateRunning   WorkloadState = "running"
	WorkloadStateCompleted WorkloadState = "completed"
	WorkloadStateFailed    WorkloadState = "failed"
	WorkloadStatePreempted WorkloadState = "preempted"
)

type Workload struct {
	ID                    string           `json:"id"`
	Type                  WorkloadType     `json:"type"`
	GPUType               string           `json:"gpu_type"`
	GPUCount              int              `json:"gpu_count"`
	Priority              WorkloadPriority `json:"priority"`
	DurationSeconds       int              `json:"duration_seconds"`
	SpotTolerant          bool             `json:"spot_tolerant"`
	Resumable             bool             `json:"resumable"`
	Replicas              int              `json:"replicas,omitempty"`
	State                 WorkloadState    `json:"state"`
	Placement             *Placement       `json:"placement,omitempty"`
	ReplicaPlacements     []Placement      `json:"replica_placements,omitempty"`
	StatusReason          string           `json:"status_reason,omitempty"`
	SchedulingExplanation string           `json:"scheduling_explanation,omitempty"`
	PreemptNoticeSeconds  int              `json:"preempt_notice_seconds,omitempty"`
	DrainStartedAt        *time.Time       `json:"drain_started_at,omitempty"`
	CheckpointState       string           `json:"checkpoint_state,omitempty"`
	ResumeEligible        bool             `json:"resume_eligible"`
	SubmittedAt           time.Time        `json:"submitted_at"`
	UpdatedAt             time.Time        `json:"updated_at"`
}

type Placement struct {
	NodeID     string `json:"node_id"`
	Region     string `json:"region,omitempty"`
	DataCenter string `json:"data_center,omitempty"`
	Zone       string `json:"zone,omitempty"`
	Provider   string `json:"provider,omitempty"`
}

func (w Workload) IsTerminal() bool {
	switch w.State {
	case WorkloadStateCompleted, WorkloadStateFailed, WorkloadStatePreempted:
		return true
	default:
		return false
	}
}

func (w Workload) IsActive() bool {
	return w.State == WorkloadStatePending || w.State == WorkloadStateRunning
}
