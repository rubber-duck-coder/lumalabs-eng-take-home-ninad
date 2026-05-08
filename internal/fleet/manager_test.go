package fleet

import (
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

func TestManagerRecordsDisruptionEvents(t *testing.T) {
	now := fixedFleetTime()
	appStore := store.NewSeededMemoryStore()
	mgr := NewManager(appStore, func() time.Time { return now })

	result, err := mgr.FailNode("node-a100-od-1")
	if err != nil {
		t.Fatalf("fail node: %v", err)
	}
	if result.Node.Health != domain.NodeHealthFailed {
		t.Fatalf("expected failed node, got %s", result.Node.Health)
	}

	events := appStore.ListEvents()
	if !hasFleetEventType(events, "node_failed") {
		t.Fatalf("expected node_failed event, got types=%v", fleetEventTypes(events))
	}
	if !hasFleetEventType(events, "workload_disrupted") {
		t.Fatalf("expected workload_disrupted event, got types=%v", fleetEventTypes(events))
	}
}

func TestSummaryUsesStoreState(t *testing.T) {
	mgr := NewManager(store.NewSeededMemoryStore(), fixedFleetTime)
	summary := mgr.Summary()

	if summary.TotalGPUs == 0 || summary.AllocatedGPUs == 0 {
		t.Fatalf("expected non-zero fleet summary, got %+v", summary)
	}
}

func fixedFleetTime() time.Time {
	return time.Date(2025, 3, 10, 16, 0, 0, 0, time.UTC)
}

func hasFleetEventType(events []domain.Event, want string) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}

func fleetEventTypes(events []domain.Event) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
