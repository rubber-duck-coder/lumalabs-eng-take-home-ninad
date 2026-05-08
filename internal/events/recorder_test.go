package events

import (
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

func TestRecorderAddsEventIDsAndPersists(t *testing.T) {
	recorder := New(store.NewMemoryStore(), fixedEventTime)

	recorder.Record("test_event", "system", "workload-1", "node-1", "hello", map[string]string{"a": "b"})

	events := recorder.store.ListEvents()
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].ID == "" {
		t.Fatal("expected generated event ID")
	}
	if events[0].Message != "hello" || events[0].Metadata["a"] != "b" {
		t.Fatalf("unexpected event payload: %+v", events[0])
	}
}

func TestRecorderRecordsAffectedWorkloads(t *testing.T) {
	recorder := New(store.NewMemoryStore(), fixedEventTime)

	recorder.RecordAffectedWorkloads("workload_disrupted", []domain.Workload{{ID: "w-1", StatusReason: "drained"}}, "node-1")

	events := recorder.store.ListEvents()
	if len(events) != 1 || events[0].WorkloadID != "w-1" {
		t.Fatalf("unexpected affected workload event: %+v", events)
	}
}

func fixedEventTime() time.Time {
	return time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
}
