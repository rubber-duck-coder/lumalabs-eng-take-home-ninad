package fleet

import (
	"testing"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
)

func TestBuildSummary(t *testing.T) {
	summary := Build(
		[]domain.Node{
			{GPUType: "A100", TotalGPUs: 8, AllocatedGPUs: 4},
			{GPUType: "H100", TotalGPUs: 16, AllocatedGPUs: 8},
		},
		[]domain.Workload{
			{State: domain.WorkloadStateRunning},
			{State: domain.WorkloadStatePending},
		},
	)

	if summary.TotalGPUs != 24 || summary.AllocatedGPUs != 12 {
		t.Fatalf("unexpected summary capacity: %+v", summary)
	}
	if summary.WorkloadsByState[domain.WorkloadStateRunning] != 1 || summary.WorkloadsByState[domain.WorkloadStatePending] != 1 {
		t.Fatalf("unexpected summary workload counts: %+v", summary.WorkloadsByState)
	}
}
