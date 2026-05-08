package telemetry

import (
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/fleet"
)

type Snapshot struct {
	Timestamp          time.Time `json:"timestamp"`
	TotalGPUs          int       `json:"total_gpus"`
	AllocatedGPUs      int       `json:"allocated_gpus"`
	AvailableGPUs      int       `json:"available_gpus"`
	FailedGPUs         int       `json:"failed_gpus"`
	UtilizationPercent float64   `json:"utilization_percent"`
	HealthyNodes       int       `json:"healthy_nodes"`
	RecoveringNodes    int       `json:"recovering_nodes"`
	FailedNodes        int       `json:"failed_nodes"`
	PendingWorkloads   int       `json:"pending_workloads"`
	RunningWorkloads   int       `json:"running_workloads"`
	CompletedWorkloads int       `json:"completed_workloads"`
	SuspendedWorkloads int       `json:"suspended_workloads"`
}

func Capture(now time.Time, nodes []domain.Node, workloads []domain.Workload) Snapshot {
	summary := fleet.Build(nodes, workloads)

	snapshot := Snapshot{
		Timestamp:          now.UTC(),
		TotalGPUs:          summary.TotalGPUs,
		AllocatedGPUs:      summary.AllocatedGPUs,
		AvailableGPUs:      summary.AvailableGPUs,
		FailedGPUs:         summary.FailedGPUs,
		UtilizationPercent: summary.Utilization,
	}

	for _, node := range nodes {
		switch node.Health {
		case domain.NodeHealthHealthy:
			snapshot.HealthyNodes++
		case domain.NodeHealthRecovering:
			snapshot.RecoveringNodes++
		case domain.NodeHealthFailed:
			snapshot.FailedNodes++
		}
	}

	for _, workload := range workloads {
		switch workload.State {
		case domain.WorkloadStatePending:
			snapshot.PendingWorkloads++
		case domain.WorkloadStateRunning:
			snapshot.RunningWorkloads++
		case domain.WorkloadStateCompleted:
			snapshot.CompletedWorkloads++
		case domain.WorkloadStatePreempted:
			snapshot.SuspendedWorkloads++
		}
	}

	return snapshot
}
