package reconciler

import (
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

func TestRunOnceHealsRecoveringNodes(t *testing.T) {
	appStore := store.NewSeededMemoryStore()
	mgr := New(appStore, fixedReconcilerTime)

	changed, err := mgr.RunOnce()
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if changed == 0 {
		t.Fatal("expected at least one node to be reconciled")
	}

	node, ok := appStore.GetNode("node-l4-od-1")
	if !ok {
		t.Fatal("expected seeded node to exist")
	}
	if node.Health != domain.NodeHealthHealthy {
		t.Fatalf("expected node to be healthy, got %s", node.Health)
	}

	events := appStore.ListEvents()
	if !hasReconcilerEvent(events, "node_reconciled") {
		t.Fatalf("expected node_reconciled event, got types=%v", reconcilerEventTypes(events))
	}
	if !hasReconcilerEvent(events, "workload_scheduled") && !hasReconcilerEvent(events, "workload_queued") {
		t.Fatalf("expected scheduling events, got types=%v", reconcilerEventTypes(events))
	}
}

func fixedReconcilerTime() time.Time {
	return time.Date(2026, time.May, 7, 13, 0, 0, 0, time.UTC)
}

func hasReconcilerEvent(events []domain.Event, want string) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}

func reconcilerEventTypes(events []domain.Event) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
