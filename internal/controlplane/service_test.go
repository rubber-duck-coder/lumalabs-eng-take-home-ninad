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

func TestTelemetryHistoryRecordsSnapshots(t *testing.T) {
	svc := New(store.NewSeededMemoryStore(), fixedControlTime)

	initial := svc.TelemetryHistory(0)
	if len(initial) == 0 {
		t.Fatal("expected initial telemetry snapshot")
	}

	if _, err := svc.SchedulerTick(); err != nil {
		t.Fatalf("scheduler tick: %v", err)
	}

	history := svc.TelemetryHistory(0)
	if len(history) < 2 {
		t.Fatalf("expected telemetry history to grow, got %d snapshot(s)", len(history))
	}

	latest := history[len(history)-1]
	if latest.TotalGPUs == 0 || latest.UtilizationPercent == 0 {
		t.Fatalf("expected populated telemetry snapshot, got %+v", latest)
	}
}

func TestTelemetryHistoryFallsBackToLiveSnapshotWhenHistoryEmpty(t *testing.T) {
	svc := New(store.NewSeededMemoryStore(), fixedControlTime)

	svc.mu.Lock()
	svc.telemetry = nil
	svc.mu.Unlock()

	history := svc.TelemetryHistory(0)
	if len(history) != 1 {
		t.Fatalf("expected a live fallback snapshot, got %d snapshot(s)", len(history))
	}
	if history[0].TotalGPUs == 0 || history[0].UtilizationPercent == 0 {
		t.Fatalf("expected populated fallback telemetry snapshot, got %+v", history[0])
	}
}

func TestRunSimulationSubmitsInferenceSpike(t *testing.T) {
	svc := New(store.NewSeededMemoryStore(), fixedControlTime)

	result, err := svc.RunSimulation("sudden-inference-spike")
	if err != nil {
		t.Fatalf("run simulation: %v", err)
	}
	if result.Scenario != "sudden-inference-spike" || len(result.Workloads) != 3 {
		t.Fatalf("unexpected simulation result: %+v", result)
	}
	if len(svc.TelemetryHistory(0)) < 2 {
		t.Fatalf("expected simulation to record telemetry history")
	}

	seenCompleted := false
	for _, event := range svc.ListEvents() {
		if event.Type == "simulation_completed" && event.Metadata["scenario"] == "sudden-inference-spike" {
			seenCompleted = true
		}
	}
	if !seenCompleted {
		t.Fatalf("expected simulation_completed event, got %+v", svc.ListEvents())
	}
}

func TestRunSimulationRejectsUnknownScenario(t *testing.T) {
	svc := New(store.NewSeededMemoryStore(), fixedControlTime)

	if _, err := svc.RunSimulation("unknown"); err == nil {
		t.Fatal("expected unknown simulation to fail")
	}
}

func TestReconcileCompletesExpiredWorkloadsAndSchedulesQueuedWork(t *testing.T) {
	now := fixedControlTime()
	memoryStore := store.NewMemoryStore()
	_, _ = memoryStore.CreateNode(domain.Node{
		ID:                 "node-a",
		GPUType:            "A100",
		TotalGPUs:          4,
		AllocatedGPUs:      4,
		CapacityClass:      domain.CapacityClassOnDemand,
		Health:             domain.NodeHealthHealthy,
		RunningWorkloadIDs: []string{"running"},
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	_, _ = memoryStore.CreateWorkload(domain.Workload{
		ID:              "running",
		Type:            domain.WorkloadTypeTraining,
		GPUType:         "A100",
		GPUCount:        4,
		Priority:        domain.WorkloadPriorityHigh,
		DurationSeconds: 1,
		State:           domain.WorkloadStateRunning,
		Placement:       &domain.Placement{NodeID: "node-a"},
		SubmittedAt:     now.Add(-2 * time.Second),
		UpdatedAt:       now.Add(-2 * time.Second),
	})
	_, _ = memoryStore.CreateWorkload(domain.Workload{
		ID:              "queued",
		Type:            domain.WorkloadTypeTraining,
		GPUType:         "A100",
		GPUCount:        4,
		Priority:        domain.WorkloadPriorityHigh,
		DurationSeconds: 5,
		State:           domain.WorkloadStatePending,
		SubmittedAt:     now,
		UpdatedAt:       now,
	})

	svc := New(memoryStore, fixedControlTime)
	changed, err := svc.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if changed == 0 {
		t.Fatal("expected reconcile to complete a workload")
	}

	completed, _ := memoryStore.GetWorkload("running")
	if completed.State != domain.WorkloadStateCompleted {
		t.Fatalf("expected expired workload completed, got %+v", completed)
	}
	scheduled, _ := memoryStore.GetWorkload("queued")
	if scheduled.State != domain.WorkloadStateRunning || scheduled.Placement == nil {
		t.Fatalf("expected queued workload to be scheduled, got %+v", scheduled)
	}
}

func fixedControlTime() time.Time {
	return time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
}
