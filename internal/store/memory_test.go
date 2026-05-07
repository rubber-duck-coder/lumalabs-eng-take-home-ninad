package store

import (
	"sync"
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
)

func TestSeededMemoryStoreNodes(t *testing.T) {
	store := NewSeededMemoryStore()

	nodes := store.ListNodes()
	if got, want := len(nodes), 6; got != want {
		t.Fatalf("expected %d seeded nodes, got %d", want, got)
	}

	first := nodes[0]
	if first.ID != "node-a100-od-1" {
		t.Fatalf("expected deterministic first node, got %s", first.ID)
	}
	if first.Region != "us-west-2" || first.DataCenter != "sfo-1" || first.Zone != "usw2-az1" {
		t.Fatalf("unexpected location fields: %+v", first)
	}
	if first.Provider != "aws" || first.CapacityClass != domain.CapacityClassOnDemand || first.Health != domain.NodeHealthHealthy {
		t.Fatalf("unexpected fleet metadata: %+v", first)
	}
}

func TestStoreReturnsCopiesAndUpdates(t *testing.T) {
	store := NewMemoryStore()

	node, err := store.CreateNode(domain.Node{
		ID:            "node-1",
		GPUType:       "A100",
		TotalGPUs:     8,
		AllocatedGPUs: 2,
		Health:        domain.NodeHealthHealthy,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	node.RunningWorkloadIDs = append(node.RunningWorkloadIDs, "mutated")
	fetched, ok := store.GetNode("node-1")
	if !ok {
		t.Fatalf("expected node to exist")
	}
	if len(fetched.RunningWorkloadIDs) != 0 {
		t.Fatalf("expected store copy to be isolated from caller mutation, got %+v", fetched.RunningWorkloadIDs)
	}

	updated, err := store.UpdateNode("node-1", func(n *domain.Node) error {
		n.AllocatedGPUs = 6
		n.RunningWorkloadIDs = []string{"w-1"}
		return nil
	})
	if err != nil {
		t.Fatalf("update node: %v", err)
	}
	if updated.AllocatedGPUs != 6 || updated.FreeGPUs() != 2 {
		t.Fatalf("unexpected updated node: %+v", updated)
	}

	workload, err := store.CreateWorkload(domain.Workload{
		ID:          "workload-1",
		Type:        domain.WorkloadTypeTraining,
		GPUType:     "A100",
		GPUCount:    4,
		Priority:    domain.WorkloadPriorityHigh,
		State:       domain.WorkloadStatePending,
		SubmittedAt: time.Date(2026, time.May, 7, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	if workload.ID != "workload-1" {
		t.Fatalf("unexpected workload: %+v", workload)
	}

	event, err := store.CreateEvent(domain.Event{
		ID:        "event-1",
		Timestamp: time.Date(2026, time.May, 7, 12, 2, 0, 0, time.UTC),
		Type:      "scheduler.assignment",
		Actor:     "scheduler",
		Message:   "placed workload on node-1",
		Metadata:  map[string]string{"node_id": "node-1"},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	event.Metadata["node_id"] = "mutated"
	fetchedEvent, ok := store.GetEvent("event-1")
	if !ok {
		t.Fatalf("expected event to exist")
	}
	if fetchedEvent.Metadata["node_id"] != "node-1" {
		t.Fatalf("expected event copy to be isolated, got %+v", fetchedEvent.Metadata)
	}

	workloads := store.ListWorkloads()
	if len(workloads) != 1 || workloads[0].ID != "workload-1" {
		t.Fatalf("unexpected workloads list: %+v", workloads)
	}

	events := store.ListEvents()
	if len(events) != 1 || events[0].ID != "event-1" {
		t.Fatalf("unexpected events list: %+v", events)
	}
}

func TestScheduleWorkloadAllocatesAtomically(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, err := store.CreateNode(domain.Node{
		ID:            "node-1",
		GPUType:       "A100",
		TotalGPUs:     4,
		AllocatedGPUs: 0,
		Health:        domain.NodeHealthHealthy,
		CapacityClass: domain.CapacityClassOnDemand,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	for _, id := range []string{"w-1", "w-2"} {
		_, err := store.CreateWorkload(domain.Workload{
			ID:          id,
			Type:        domain.WorkloadTypeInference,
			GPUType:     "A100",
			GPUCount:    3,
			Priority:    domain.WorkloadPriorityNormal,
			State:       domain.WorkloadStatePending,
			SubmittedAt: now,
			UpdatedAt:   now,
		})
		if err != nil {
			t.Fatalf("create workload %s: %v", id, err)
		}
	}

	var wg sync.WaitGroup
	for _, id := range []string{"w-1", "w-2"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, _ = store.ScheduleWorkload(id, now.Add(time.Second))
		}(id)
	}
	wg.Wait()

	node, ok := store.GetNode("node-1")
	if !ok {
		t.Fatal("expected node")
	}
	if node.AllocatedGPUs > node.TotalGPUs {
		t.Fatalf("node over-allocated: %+v", node)
	}

	running := 0
	pending := 0
	for _, workload := range store.ListWorkloads() {
		switch workload.State {
		case domain.WorkloadStateRunning:
			running++
		case domain.WorkloadStatePending:
			pending++
		}
	}
	if running != 1 || pending != 1 {
		t.Fatalf("expected one running and one pending workload, got running=%d pending=%d", running, pending)
	}
}
