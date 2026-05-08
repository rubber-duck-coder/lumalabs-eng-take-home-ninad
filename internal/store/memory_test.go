package store

import (
	"sync"
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/scheduler"
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

func TestSeedDemoDataPopulatesDeterministicFixture(t *testing.T) {
	store := NewMemoryStore()

	summary, err := store.SeedDemoData()
	if err != nil {
		t.Fatalf("seed demo data: %v", err)
	}
	if summary.Nodes != 6 || summary.Workloads != 3 || summary.Events != 2 {
		t.Fatalf("unexpected seed summary: %+v", summary)
	}

	nodes := store.ListNodes()
	if len(nodes) != 6 {
		t.Fatalf("expected 6 seeded nodes, got %d", len(nodes))
	}
	workloads := store.ListWorkloads()
	if len(workloads) != 3 {
		t.Fatalf("expected 3 seeded workloads, got %d", len(workloads))
	}
	events := store.ListEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 seeded events, got %d", len(events))
	}

	var running, pending int
	for _, workload := range workloads {
		switch workload.State {
		case domain.WorkloadStateRunning:
			running++
		case domain.WorkloadStatePending:
			pending++
		}
	}
	if running != 1 || pending != 2 {
		t.Fatalf("expected one running and two pending workloads, got running=%d pending=%d", running, pending)
	}
}

func TestClearRemovesAllDemoData(t *testing.T) {
	store := NewSeededMemoryStore()

	summary, err := store.Clear()
	if err != nil {
		t.Fatalf("clear demo data: %v", err)
	}
	if summary.Nodes != 6 || summary.Workloads != 3 || summary.Events != 2 {
		t.Fatalf("unexpected clear summary: %+v", summary)
	}

	if nodes := store.ListNodes(); len(nodes) != 0 {
		t.Fatalf("expected no nodes after clear, got %d", len(nodes))
	}
	if workloads := store.ListWorkloads(); len(workloads) != 0 {
		t.Fatalf("expected no workloads after clear, got %d", len(workloads))
	}
	if events := store.ListEvents(); len(events) != 0 {
		t.Fatalf("expected no events after clear, got %d", len(events))
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

func TestSchedulePendingWorkloadsOrdersByPriorityThenSubmission(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, err := store.CreateNode(domain.Node{
		ID:            "node-a100",
		GPUType:       "A100",
		TotalGPUs:     8,
		AllocatedGPUs: 0,
		Health:        domain.NodeHealthHealthy,
		CapacityClass: domain.CapacityClassOnDemand,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	_, _ = store.CreateWorkload(domain.Workload{
		ID:          "low-older",
		Type:        domain.WorkloadTypeTraining,
		GPUType:     "A100",
		GPUCount:    2,
		Priority:    domain.WorkloadPriorityLow,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	_, _ = store.CreateWorkload(domain.Workload{
		ID:          "high-newer",
		Type:        domain.WorkloadTypeTraining,
		GPUType:     "A100",
		GPUCount:    2,
		Priority:    domain.WorkloadPriorityHigh,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now.Add(2 * time.Minute),
		UpdatedAt:   now,
	})
	_, _ = store.CreateWorkload(domain.Workload{
		ID:          "high-older",
		Type:        domain.WorkloadTypeTraining,
		GPUType:     "A100",
		GPUCount:    2,
		Priority:    domain.WorkloadPriorityHigh,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now.Add(1 * time.Minute),
		UpdatedAt:   now,
	})

	results, err := store.SchedulePendingWorkloads(now.Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("schedule pending: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 scheduling results, got %d", len(results))
	}

	gotOrder := []string{results[0].Workload.ID, results[1].Workload.ID, results[2].Workload.ID}
	wantOrder := []string{"high-older", "high-newer", "low-older"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("unexpected scheduling order: got=%v want=%v", gotOrder, wantOrder)
		}
		if results[i].Decision.Outcome != scheduler.OutcomePlaced {
			t.Fatalf("expected placed decision for %s, got %s", results[i].Workload.ID, results[i].Decision.Outcome)
		}
	}
}

func TestFailNodeFreesAllocationAndRequeuesWorkloads(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, _ = store.CreateNode(domain.Node{
		ID:                 "node-a",
		GPUType:            "A100",
		TotalGPUs:          4,
		AllocatedGPUs:      4,
		Health:             domain.NodeHealthHealthy,
		CapacityClass:      domain.CapacityClassOnDemand,
		RunningWorkloadIDs: []string{"w-1"},
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	_, _ = store.CreateNode(domain.Node{
		ID:            "node-b",
		GPUType:       "A100",
		TotalGPUs:     4,
		AllocatedGPUs: 0,
		Health:        domain.NodeHealthHealthy,
		CapacityClass: domain.CapacityClassOnDemand,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	_, _ = store.CreateWorkload(domain.Workload{
		ID:       "w-1",
		Type:     domain.WorkloadTypeTraining,
		GPUType:  "A100",
		GPUCount: 4,
		Priority: domain.WorkloadPriorityHigh,
		State:    domain.WorkloadStateRunning,
		Placement: &domain.Placement{
			NodeID: "node-a",
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})

	result, err := store.FailNode("node-a", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("fail node: %v", err)
	}
	if result.Node.Health != domain.NodeHealthFailed || result.Node.AllocatedGPUs != 0 {
		t.Fatalf("unexpected failed node state: %+v", result.Node)
	}
	if len(result.AffectedWorkloads) != 1 {
		t.Fatalf("expected one affected workload, got %d", len(result.AffectedWorkloads))
	}
	if len(result.Scheduled) != 1 || result.Scheduled[0].Workload.ID != "w-1" {
		t.Fatalf("expected workload to be rescheduled once, got %+v", result.Scheduled)
	}
	if result.Scheduled[0].Workload.State != domain.WorkloadStateRunning || result.Scheduled[0].Workload.Placement == nil || result.Scheduled[0].Workload.Placement.NodeID != "node-b" {
		t.Fatalf("expected workload running on node-b, got %+v", result.Scheduled[0].Workload)
	}
}

func TestFailNodeEvictsWorkloadsByPlacementWhenRunningListIsStale(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, _ = store.CreateNode(domain.Node{
		ID:            "node-a",
		GPUType:       "A100",
		TotalGPUs:     4,
		AllocatedGPUs: 4,
		Health:        domain.NodeHealthHealthy,
		CapacityClass: domain.CapacityClassOnDemand,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	_, _ = store.CreateWorkload(domain.Workload{
		ID:       "w-stale",
		Type:     domain.WorkloadTypeTraining,
		GPUType:  "A100",
		GPUCount: 4,
		Priority: domain.WorkloadPriorityHigh,
		State:    domain.WorkloadStateRunning,
		Placement: &domain.Placement{
			NodeID: "node-a",
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})

	result, err := store.FailNode("node-a", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("fail node: %v", err)
	}
	if len(result.AffectedWorkloads) != 1 || result.AffectedWorkloads[0].ID != "w-stale" {
		t.Fatalf("expected stale running workload to be affected, got %+v", result.AffectedWorkloads)
	}
	workload, _ := store.GetWorkload("w-stale")
	if workload.State != domain.WorkloadStatePending || workload.Placement != nil {
		t.Fatalf("expected workload requeued after node failure, got %+v", workload)
	}
}

func TestRecoverNodeSetsHealthyAndSchedulesPending(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, _ = store.CreateNode(domain.Node{
		ID:            "node-1",
		GPUType:       "A100",
		TotalGPUs:     4,
		AllocatedGPUs: 0,
		Health:        domain.NodeHealthRecovering,
		CapacityClass: domain.CapacityClassOnDemand,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	_, _ = store.CreateWorkload(domain.Workload{
		ID:          "w-pending",
		Type:        domain.WorkloadTypeTraining,
		GPUType:     "A100",
		GPUCount:    2,
		Priority:    domain.WorkloadPriorityHigh,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now,
		UpdatedAt:   now,
	})

	result, err := store.RecoverNode("node-1", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("recover node: %v", err)
	}
	if result.Node.Health != domain.NodeHealthHealthy {
		t.Fatalf("expected healthy node after recover, got %+v", result.Node)
	}
	if len(result.Scheduled) != 1 || result.Scheduled[0].Workload.ID != "w-pending" {
		t.Fatalf("expected one scheduled workload after recover, got %+v", result.Scheduled)
	}
}

func TestPreemptSpotNodePreemptsRunningWorkloads(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, _ = store.CreateNode(domain.Node{
		ID:                 "spot-1",
		GPUType:            "L4",
		TotalGPUs:          4,
		AllocatedGPUs:      2,
		Health:             domain.NodeHealthHealthy,
		CapacityClass:      domain.CapacityClassSpot,
		RunningWorkloadIDs: []string{"w-spot"},
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	_, _ = store.CreateWorkload(domain.Workload{
		ID:       "w-spot",
		Type:     domain.WorkloadTypeInference,
		GPUType:  "L4",
		GPUCount: 2,
		Priority: domain.WorkloadPriorityNormal,
		State:    domain.WorkloadStateRunning,
		Placement: &domain.Placement{
			NodeID: "spot-1",
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})

	result, err := store.PreemptSpotNode("spot-1", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("preempt spot node: %v", err)
	}
	if result.Node.Health != domain.NodeHealthFailed || result.Node.AllocatedGPUs != 0 {
		t.Fatalf("unexpected spot node after preemption: %+v", result.Node)
	}
	if len(result.AffectedWorkloads) != 1 {
		t.Fatalf("expected one affected workload, got %d", len(result.AffectedWorkloads))
	}
	if result.AffectedWorkloads[0].State != domain.WorkloadStatePending {
		t.Fatalf("expected pending workload after preemption, got %+v", result.AffectedWorkloads[0])
	}
	if result.AffectedWorkloads[0].Placement != nil {
		t.Fatalf("expected cleared placement, got %+v", result.AffectedWorkloads[0])
	}
	if len(result.Scheduled) != 1 {
		t.Fatalf("expected one scheduling result after preemption, got %+v", result.Scheduled)
	}
}
