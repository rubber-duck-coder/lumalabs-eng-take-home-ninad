package workloads

import (
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

func TestSubmitRecordsAndSchedules(t *testing.T) {
	now := fixedWorkloadTime()
	appStore := store.NewSeededMemoryStore()
	mgr := New(appStore, func() time.Time { return now })

	workload, err := mgr.Submit(SubmitRequest{
		Type:            domain.WorkloadTypeTraining,
		GPUType:         "a100",
		GPUCount:        1,
		Priority:        domain.WorkloadPriorityHigh,
		DurationSeconds: 600,
		SpotTolerant:    false,
	})
	if err != nil {
		t.Fatalf("submit workload: %v", err)
	}

	if workload.State != domain.WorkloadStateRunning {
		t.Fatalf("expected running workload, got %s", workload.State)
	}
	if workload.GPUType != "A100" {
		t.Fatalf("expected normalized GPU type, got %q", workload.GPUType)
	}

	events := appStore.ListEvents()
	if !hasEventType(events, "workload_submitted") {
		t.Fatalf("expected submit event, got types=%v", eventTypes(events))
	}
	if !hasEventType(events, "workload_scheduled") {
		t.Fatalf("expected scheduled event, got types=%v", eventTypes(events))
	}
}

func TestSchedulerTickRecordsPendingResults(t *testing.T) {
	now := fixedWorkloadTime()
	appStore := store.NewSeededMemoryStore()
	mgr := New(appStore, func() time.Time { return now })

	_, err := appStore.CreateWorkload(domain.Workload{
		ID:              "w-pending",
		Type:            domain.WorkloadTypeInference,
		GPUType:         "A100",
		GPUCount:        99,
		Priority:        domain.WorkloadPriorityNormal,
		DurationSeconds: 900,
		State:           domain.WorkloadStatePending,
		SubmittedAt:     now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}

	results, err := mgr.SchedulerTick()
	if err != nil {
		t.Fatalf("scheduler tick: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected scheduling results")
	}

	events := appStore.ListEvents()
	if !hasEventType(events, "scheduler_tick") {
		t.Fatalf("expected scheduler tick event, got types=%v", eventTypes(events))
	}
	if !hasEventType(events, "workload_queued") {
		t.Fatalf("expected queued event, got types=%v", eventTypes(events))
	}
}

func fixedWorkloadTime() time.Time {
	return time.Date(2025, 3, 10, 15, 4, 5, 0, time.UTC)
}

func hasEventType(events []domain.Event, want string) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}

func eventTypes(events []domain.Event) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
