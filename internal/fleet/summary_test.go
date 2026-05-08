package fleet

import (
	"testing"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
)

func TestBuildSummary(t *testing.T) {
	summary := Build(
		[]domain.Node{
			{GPUType: "A100", TotalGPUs: 8, AllocatedGPUs: 4, Health: domain.NodeHealthHealthy},
			{GPUType: "H100", TotalGPUs: 16, AllocatedGPUs: 8, Health: domain.NodeHealthHealthy},
			{GPUType: "L4", TotalGPUs: 4, AllocatedGPUs: 0, Health: domain.NodeHealthFailed},
		},
		[]domain.Workload{
			{State: domain.WorkloadStateRunning},
			{State: domain.WorkloadStatePending},
		},
	)

	if summary.TotalGPUs != 28 || summary.AllocatedGPUs != 12 {
		t.Fatalf("unexpected summary capacity: %+v", summary)
	}
	if summary.AvailableGPUs != 12 || summary.FailedGPUs != 4 {
		t.Fatalf("expected available capacity to exclude failed GPUs, got %+v", summary)
	}
	if summary.WorkloadsByState[domain.WorkloadStateRunning] != 1 || summary.WorkloadsByState[domain.WorkloadStatePending] != 1 {
		t.Fatalf("unexpected summary workload counts: %+v", summary.WorkloadsByState)
	}
}
