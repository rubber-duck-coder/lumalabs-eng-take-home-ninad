package store

import (
	"strings"
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

func TestInferenceWorkloadSchedulesAcrossDistinctNodes(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	for _, id := range []string{"node-a", "node-b", "node-c"} {
		_, err := store.CreateNode(domain.Node{
			ID:            id,
			GPUType:       "A100",
			TotalGPUs:     2,
			AllocatedGPUs: 0,
			Health:        domain.NodeHealthHealthy,
			CapacityClass: domain.CapacityClassOnDemand,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if err != nil {
			t.Fatalf("create node %s: %v", id, err)
		}
	}

	_, err := store.CreateWorkload(domain.Workload{
		ID:          "svc-1",
		Type:        domain.WorkloadTypeInference,
		GPUType:     "A100",
		GPUCount:    1,
		Priority:    domain.WorkloadPriorityNormal,
		Replicas:    3,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create inference workload: %v", err)
	}

	result, err := store.ScheduleWorkload("svc-1", now.Add(time.Second))
	if err != nil {
		t.Fatalf("schedule inference workload: %v", err)
	}
	if result.Decision.Outcome != scheduler.OutcomePlaced {
		t.Fatalf("expected placed outcome, got %+v", result.Decision)
	}
	if result.Workload.State != domain.WorkloadStateRunning {
		t.Fatalf("expected running workload, got %+v", result.Workload)
	}
	if len(result.Workload.ReplicaPlacements) != 3 {
		t.Fatalf("expected 3 replica placements, got %+v", result.Workload.ReplicaPlacements)
	}

	nodesByID := make(map[string]struct{})
	for _, placement := range result.Workload.ReplicaPlacements {
		nodesByID[placement.NodeID] = struct{}{}
	}
	if len(nodesByID) != 3 {
		t.Fatalf("expected placements across 3 distinct nodes, got %+v", result.Workload.ReplicaPlacements)
	}
	if result.Workload.Placement == nil {
		t.Fatal("expected canonical placement")
	}
	if len(result.Workload.ReplicaPlacements) != result.Workload.Replicas {
		t.Fatalf("expected replicas and placements to match, got replicas=%d placements=%d", result.Workload.Replicas, len(result.Workload.ReplicaPlacements))
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
		ID:        "w-spot",
		Type:      domain.WorkloadTypeInference,
		GPUType:   "L4",
		GPUCount:  2,
		Priority:  domain.WorkloadPriorityNormal,
		Resumable: true,
		State:     domain.WorkloadStateRunning,
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
	if !result.AffectedWorkloads[0].ResumeEligible {
		t.Fatalf("expected resumable workload to remain resume eligible, got %+v", result.AffectedWorkloads[0])
	}
	if result.AffectedWorkloads[0].PreemptNoticeSeconds != 30 {
		t.Fatalf("expected preempt notice to be recorded, got %+v", result.AffectedWorkloads[0])
	}
	if result.AffectedWorkloads[0].CheckpointState != "checkpointed" {
		t.Fatalf("expected checkpoint state to be recorded, got %+v", result.AffectedWorkloads[0])
	}
	if len(result.Scheduled) != 1 {
		t.Fatalf("expected one scheduling result after preemption, got %+v", result.Scheduled)
	}
}

func TestFailNodePreservesRemainingInferenceReplicas(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	for _, id := range []string{"node-a", "node-b"} {
		_, err := store.CreateNode(domain.Node{
			ID:            id,
			GPUType:       "A100",
			TotalGPUs:     1,
			AllocatedGPUs: 0,
			Health:        domain.NodeHealthHealthy,
			CapacityClass: domain.CapacityClassOnDemand,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if err != nil {
			t.Fatalf("create node %s: %v", id, err)
		}
	}

	_, err := store.CreateWorkload(domain.Workload{
		ID:          "svc-fail",
		Type:        domain.WorkloadTypeInference,
		GPUType:     "A100",
		GPUCount:    1,
		Priority:    domain.WorkloadPriorityNormal,
		Replicas:    2,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create inference workload: %v", err)
	}
	if _, err := store.ScheduleWorkload("svc-fail", now.Add(time.Second)); err != nil {
		t.Fatalf("schedule inference workload: %v", err)
	}

	result, err := store.FailNode("node-a", now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("fail node: %v", err)
	}
	if result.Node.Health != domain.NodeHealthFailed {
		t.Fatalf("expected failed node, got %+v", result.Node)
	}
	if len(result.Scheduled) != 0 {
		t.Fatalf("expected no reschedule when one replica remains, got %+v", result.Scheduled)
	}

	workload, ok := store.GetWorkload("svc-fail")
	if !ok {
		t.Fatal("expected workload")
	}
	if workload.State != domain.WorkloadStateRunning {
		t.Fatalf("expected workload to remain running, got %+v", workload)
	}
	if len(workload.ReplicaPlacements) != 1 {
		t.Fatalf("expected one remaining replica placement, got %+v", workload.ReplicaPlacements)
	}
	if workload.ReplicaPlacements[0].NodeID != "node-b" {
		t.Fatalf("expected surviving replica on node-b, got %+v", workload.ReplicaPlacements)
	}
}

func TestScheduleWorkloadPreemptsInferenceReplicaAndKeepsServiceRunning(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	for _, id := range []string{"node-a", "node-b"} {
		_, err := store.CreateNode(domain.Node{
			ID:            id,
			GPUType:       "A100",
			TotalGPUs:     2,
			AllocatedGPUs: 0,
			Health:        domain.NodeHealthHealthy,
			CapacityClass: domain.CapacityClassOnDemand,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if err != nil {
			t.Fatalf("create node %s: %v", id, err)
		}
	}

	_, err := store.CreateWorkload(domain.Workload{
		ID:          "svc-preempt",
		Type:        domain.WorkloadTypeInference,
		GPUType:     "A100",
		GPUCount:    1,
		Priority:    domain.WorkloadPriorityLow,
		Replicas:    2,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create inference workload: %v", err)
	}
	if _, err := store.ScheduleWorkload("svc-preempt", now.Add(time.Second)); err != nil {
		t.Fatalf("schedule inference workload: %v", err)
	}

	_, err = store.CreateWorkload(domain.Workload{
		ID:          "high-1",
		Type:        domain.WorkloadTypeTraining,
		GPUType:     "A100",
		GPUCount:    2,
		Priority:    domain.WorkloadPriorityHigh,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now.Add(2 * time.Minute),
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create high priority workload: %v", err)
	}

	result, err := store.ScheduleWorkload("high-1", now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("schedule high priority workload: %v", err)
	}
	if result.Decision.Outcome != scheduler.OutcomePlaced {
		t.Fatalf("expected placed outcome, got %+v", result.Decision)
	}

	inference, ok := store.GetWorkload("svc-preempt")
	if !ok {
		t.Fatal("expected inference workload")
	}
	if inference.State != domain.WorkloadStateRunning {
		t.Fatalf("expected inference service to remain running, got %+v", inference)
	}
	if len(inference.ReplicaPlacements) != 1 {
		t.Fatalf("expected one surviving replica placement, got %+v", inference.ReplicaPlacements)
	}
	if inference.ReplicaPlacements[0].NodeID != "node-b" {
		t.Fatalf("expected surviving replica on node-b, got %+v", inference.ReplicaPlacements)
	}

	high, ok := store.GetWorkload("high-1")
	if !ok {
		t.Fatal("expected high priority workload")
	}
	if high.State != domain.WorkloadStateRunning || high.Placement == nil || high.Placement.NodeID != "node-a" {
		t.Fatalf("expected high priority workload on node-a, got %+v", high)
	}
}

func TestScheduleWorkloadPreemptsLowerPriorityRunningWorkload(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, err := store.CreateNode(domain.Node{
		ID:                 "node-1",
		GPUType:            "A100",
		TotalGPUs:          4,
		AllocatedGPUs:      4,
		Health:             domain.NodeHealthHealthy,
		CapacityClass:      domain.CapacityClassOnDemand,
		RunningWorkloadIDs: []string{"low-1"},
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	_, err = store.CreateWorkload(domain.Workload{
		ID:       "low-1",
		Type:     domain.WorkloadTypeTraining,
		GPUType:  "A100",
		GPUCount: 4,
		Priority: domain.WorkloadPriorityLow,
		State:    domain.WorkloadStateRunning,
		Placement: &domain.Placement{
			NodeID: "node-1",
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create low workload: %v", err)
	}
	_, err = store.CreateWorkload(domain.Workload{
		ID:          "high-1",
		Type:        domain.WorkloadTypeTraining,
		GPUType:     "A100",
		GPUCount:    4,
		Priority:    domain.WorkloadPriorityHigh,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now.Add(time.Minute),
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create high workload: %v", err)
	}

	result, err := store.ScheduleWorkload("high-1", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("schedule high workload: %v", err)
	}
	if result.Decision.Outcome != scheduler.OutcomePlaced {
		t.Fatalf("expected placed outcome, got %+v", result.Decision)
	}
	if !strings.Contains(result.Decision.Reason, "preempted 1 lower-priority workload") {
		t.Fatalf("expected preemption reason, got %q", result.Decision.Reason)
	}

	high, ok := store.GetWorkload("high-1")
	if !ok {
		t.Fatal("expected high workload")
	}
	if high.State != domain.WorkloadStateRunning || high.Placement == nil || high.Placement.NodeID != "node-1" {
		t.Fatalf("expected high workload to run on node-1, got %+v", high)
	}

	low, ok := store.GetWorkload("low-1")
	if !ok {
		t.Fatal("expected low workload")
	}
	if low.State != domain.WorkloadStatePending || low.Placement != nil {
		t.Fatalf("expected low workload to be requeued, got %+v", low)
	}
	if low.StatusReason == "" || !strings.Contains(low.StatusReason, "preempted") {
		t.Fatalf("expected low workload to record preemption reason, got %+v", low)
	}

	node, ok := store.GetNode("node-1")
	if !ok {
		t.Fatal("expected node")
	}
	if node.AllocatedGPUs != 4 || len(node.RunningWorkloadIDs) != 1 || node.RunningWorkloadIDs[0] != "high-1" {
		t.Fatalf("expected node to run only the high-priority workload, got %+v", node)
	}
}

func TestScheduleWorkloadDoesNotPreemptEqualPriorityRunningWorkload(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, err := store.CreateNode(domain.Node{
		ID:                 "node-1",
		GPUType:            "A100",
		TotalGPUs:          4,
		AllocatedGPUs:      4,
		Health:             domain.NodeHealthHealthy,
		CapacityClass:      domain.CapacityClassOnDemand,
		RunningWorkloadIDs: []string{"normal-1"},
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	_, err = store.CreateWorkload(domain.Workload{
		ID:       "normal-1",
		Type:     domain.WorkloadTypeTraining,
		GPUType:  "A100",
		GPUCount: 4,
		Priority: domain.WorkloadPriorityNormal,
		State:    domain.WorkloadStateRunning,
		Placement: &domain.Placement{
			NodeID: "node-1",
		},
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create running workload: %v", err)
	}
	_, err = store.CreateWorkload(domain.Workload{
		ID:          "normal-2",
		Type:        domain.WorkloadTypeTraining,
		GPUType:     "A100",
		GPUCount:    4,
		Priority:    domain.WorkloadPriorityNormal,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now.Add(time.Minute),
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create pending workload: %v", err)
	}

	result, err := store.ScheduleWorkload("normal-2", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("schedule workload: %v", err)
	}
	if result.Decision.Outcome != scheduler.OutcomeQueued {
		t.Fatalf("expected queued outcome, got %+v", result.Decision)
	}

	running, _ := store.GetWorkload("normal-1")
	if running.State != domain.WorkloadStateRunning || running.Placement == nil || running.Placement.NodeID != "node-1" {
		t.Fatalf("expected existing workload to remain running, got %+v", running)
	}
	pending, _ := store.GetWorkload("normal-2")
	if pending.State != domain.WorkloadStatePending || pending.Placement != nil {
		t.Fatalf("expected pending workload to stay queued, got %+v", pending)
	}
}

func TestScheduleInferenceWorkloadUsesDistinctReplicaPlacements(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	for _, id := range []string{"node-a", "node-b"} {
		_, err := store.CreateNode(domain.Node{
			ID:            id,
			GPUType:       "A100",
			TotalGPUs:     4,
			AllocatedGPUs: 0,
			Health:        domain.NodeHealthHealthy,
			CapacityClass: domain.CapacityClassOnDemand,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if err != nil {
			t.Fatalf("create node %s: %v", id, err)
		}
	}

	_, err := store.CreateWorkload(domain.Workload{
		ID:          "inference-svc",
		Type:        domain.WorkloadTypeInference,
		GPUType:     "A100",
		GPUCount:    1,
		Priority:    domain.WorkloadPriorityNormal,
		Replicas:    2,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create inference workload: %v", err)
	}

	result, err := store.ScheduleWorkload("inference-svc", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("schedule inference workload: %v", err)
	}
	if result.Decision.Outcome != scheduler.OutcomePlaced {
		t.Fatalf("expected placed outcome, got %+v", result.Decision)
	}
	if got := len(result.Workload.ReplicaPlacements); got != 2 {
		t.Fatalf("expected 2 replica placements, got %d", got)
	}
	if result.Workload.ReplicaPlacements[0].NodeID == result.Workload.ReplicaPlacements[1].NodeID {
		t.Fatalf("expected replica placements on distinct nodes, got %+v", result.Workload.ReplicaPlacements)
	}

	first, _ := store.GetNode("node-a")
	second, _ := store.GetNode("node-b")
	if first.AllocatedGPUs != 1 || second.AllocatedGPUs != 1 {
		t.Fatalf("expected one gpu allocated on each node, got first=%+v second=%+v", first, second)
	}

	workload, ok := store.GetWorkload("inference-svc")
	if !ok {
		t.Fatal("expected inference workload")
	}
	if workload.State != domain.WorkloadStateRunning || len(workload.ReplicaPlacements) != 2 {
		t.Fatalf("expected running workload with replica placements, got %+v", workload)
	}
}

func TestScheduleInferenceWorkloadQueuesWhenNotEnoughDistinctNodes(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	_, err := store.CreateNode(domain.Node{
		ID:            "node-a",
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

	_, err = store.CreateWorkload(domain.Workload{
		ID:          "inference-svc",
		Type:        domain.WorkloadTypeInference,
		GPUType:     "A100",
		GPUCount:    1,
		Priority:    domain.WorkloadPriorityNormal,
		Replicas:    2,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create inference workload: %v", err)
	}

	result, err := store.ScheduleWorkload("inference-svc", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("schedule inference workload: %v", err)
	}
	if result.Decision.Outcome != scheduler.OutcomeQueued {
		t.Fatalf("expected queued outcome, got %+v", result.Decision)
	}

	workload, ok := store.GetWorkload("inference-svc")
	if !ok {
		t.Fatal("expected inference workload")
	}
	if workload.State != domain.WorkloadStatePending || len(workload.ReplicaPlacements) != 0 {
		t.Fatalf("expected queued workload without partial placements, got %+v", workload)
	}
}
