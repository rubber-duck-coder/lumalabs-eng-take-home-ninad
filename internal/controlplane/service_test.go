package controlplane

import (
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

func TestSubmitWorkloadRecordsEventsAndSchedules(t *testing.T) {
	svc := New(store.NewSeededMemoryStore(), fixedControlTime)

	workload, err := svc.SubmitWorkload(SubmitWorkloadRequest{
		Type:            domain.WorkloadTypeBatch,
		GPUType:         "A100",
		GPUCount:        2,
		Priority:        domain.WorkloadPriorityNormal,
		DurationSeconds: 300,
		SpotTolerant:    true,
	})
	if err != nil {
		t.Fatalf("submit workload: %v", err)
	}
	if workload.State != domain.WorkloadStateRunning {
		t.Fatalf("expected running workload, got %+v", workload)
	}
	if workload.Placement == nil {
		t.Fatal("expected placement")
	}

	events := svc.ListEvents()
	if len(events) < 2 {
		t.Fatalf("expected submission and scheduling events, got %d", len(events))
	}
}

func TestFleetSummaryUsesServiceBoundary(t *testing.T) {
	svc := New(store.NewSeededMemoryStore(), fixedControlTime)

	summary := svc.FleetSummary()
	if summary.TotalGPUs == 0 || summary.Utilization == 0 {
		t.Fatalf("expected non-empty fleet summary, got %+v", summary)
	}
	if summary.WorkloadsByState[domain.WorkloadStateRunning] == 0 {
		t.Fatalf("expected running workloads in summary, got %+v", summary.WorkloadsByState)
	}
}

func fixedControlTime() time.Time {
	return time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
}
