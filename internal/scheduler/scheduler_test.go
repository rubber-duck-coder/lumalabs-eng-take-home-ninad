package scheduler

import (
	"testing"
	"time"
)

func TestWorkloadOrderingHelper(t *testing.T) {
	base := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
	workloads := []Workload{
		{ID: "c", Priority: PriorityNormal, SubmittedAt: base.Add(3 * time.Minute)},
		{ID: "b", Priority: PriorityHigh, SubmittedAt: base.Add(2 * time.Minute)},
		{ID: "a", Priority: PriorityHigh, SubmittedAt: base.Add(2 * time.Minute)},
		{ID: "d", Priority: PriorityLow, SubmittedAt: base},
	}

	OrderPendingWorkloads(workloads)

	got := []string{workloads[0].ID, workloads[1].ID, workloads[2].ID, workloads[3].ID}
	want := []string{"a", "b", "c", "d"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order: got %v want %v", got, want)
		}
	}
}

func TestWorkloadOrderingPrefersInferenceThenNonSpotWithinPriority(t *testing.T) {
	base := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
	workloads := []Workload{
		{ID: "batch-spot", Type: WorkloadTypeBatch, Priority: PriorityNormal, SubmittedAt: base, SpotTolerant: true},
		{ID: "batch-ondemand", Type: WorkloadTypeBatch, Priority: PriorityNormal, SubmittedAt: base.Add(time.Minute)},
		{ID: "training", Type: WorkloadTypeTraining, Priority: PriorityNormal, SubmittedAt: base.Add(2 * time.Minute)},
		{ID: "inference-spot", Type: WorkloadTypeInference, Priority: PriorityNormal, SubmittedAt: base.Add(3 * time.Minute), SpotTolerant: true},
		{ID: "inference-ondemand", Type: WorkloadTypeInference, Priority: PriorityNormal, SubmittedAt: base.Add(4 * time.Minute)},
	}

	OrderPendingWorkloads(workloads)

	got := []string{workloads[0].ID, workloads[1].ID, workloads[2].ID, workloads[3].ID, workloads[4].ID}
	want := []string{"inference-ondemand", "inference-spot", "training", "batch-ondemand", "batch-spot"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected class-aware order: got %v want %v", got, want)
		}
	}
}

func TestResourceFitQueuesWhenNoEligibleNodes(t *testing.T) {
	workload := Workload{
		ID:           "wl-1",
		Type:         WorkloadTypeTraining,
		GPUType:      "H100",
		GPUCount:     2,
		Priority:     PriorityHigh,
		SubmittedAt:  time.Unix(100, 0),
		SpotTolerant: true,
	}

	nodes := []Node{
		{ID: "node-a", GPUType: "H100", TotalGPUs: 1, AllocatedGPUs: 0, CapacityClass: CapacityClassOnDemand, Health: NodeHealthHealthy},
		{ID: "node-b", GPUType: "A100", TotalGPUs: 8, AllocatedGPUs: 0, CapacityClass: CapacityClassOnDemand, Health: NodeHealthHealthy},
		{ID: "node-c", GPUType: "H100", TotalGPUs: 8, AllocatedGPUs: 0, CapacityClass: CapacityClassSpot, Health: NodeHealthFailed},
	}

	decision := Decide(workload, nodes)

	if decision.Outcome != OutcomeQueued {
		t.Fatalf("expected queued outcome, got %s", decision.Outcome)
	}
	if decision.SelectedNode != nil {
		t.Fatalf("expected no selected node, got %#v", decision.SelectedNode)
	}
	if len(decision.RejectedNodes) != 3 {
		t.Fatalf("expected 3 rejected nodes, got %d", len(decision.RejectedNodes))
	}
	if decision.Reason == "" {
		t.Fatalf("expected queue reason")
	}
}

func TestBatchSpotPreferenceAndInferenceOnDemandPreference(t *testing.T) {
	batch := Workload{
		ID:           "batch-1",
		Type:         WorkloadTypeBatch,
		GPUType:      "L4",
		GPUCount:     1,
		Priority:     PriorityNormal,
		SubmittedAt:  time.Unix(200, 0),
		SpotTolerant: true,
	}
	inference := Workload{
		ID:           "infer-1",
		Type:         WorkloadTypeInference,
		GPUType:      "L4",
		GPUCount:     1,
		Priority:     PriorityNormal,
		SubmittedAt:  time.Unix(200, 0),
		SpotTolerant: true,
	}

	nodes := []Node{
		{ID: "ondemand", GPUType: "L4", TotalGPUs: 4, AllocatedGPUs: 0, CapacityClass: CapacityClassOnDemand, Health: NodeHealthHealthy},
		{ID: "spot", GPUType: "L4", TotalGPUs: 4, AllocatedGPUs: 0, CapacityClass: CapacityClassSpot, Health: NodeHealthHealthy},
	}

	batchDecision := Decide(batch, nodes)
	if batchDecision.Outcome != OutcomePlaced {
		t.Fatalf("expected batch to be placed, got %s (%s)", batchDecision.Outcome, batchDecision.Reason)
	}
	if batchDecision.SelectedNode == nil || batchDecision.SelectedNode.ID != "spot" {
		t.Fatalf("expected batch workload to prefer spot, got %#v", batchDecision.SelectedNode)
	}

	inferenceDecision := Decide(inference, nodes)
	if inferenceDecision.Outcome != OutcomePlaced {
		t.Fatalf("expected inference to be placed, got %s (%s)", inferenceDecision.Outcome, inferenceDecision.Reason)
	}
	if inferenceDecision.SelectedNode == nil || inferenceDecision.SelectedNode.ID != "ondemand" {
		t.Fatalf("expected inference workload to prefer on-demand, got %#v", inferenceDecision.SelectedNode)
	}
}

func TestInferencePrefersLessUtilizedOnDemandNode(t *testing.T) {
	workload := Workload{
		ID:           "infer-spread",
		Type:         WorkloadTypeInference,
		GPUType:      "A100",
		GPUCount:     1,
		Priority:     PriorityNormal,
		SubmittedAt:  time.Unix(250, 0),
		SpotTolerant: true,
	}

	nodes := []Node{
		{ID: "packed", GPUType: "A100", TotalGPUs: 8, AllocatedGPUs: 6, CapacityClass: CapacityClassOnDemand, Health: NodeHealthHealthy},
		{ID: "spare", GPUType: "A100", TotalGPUs: 8, AllocatedGPUs: 1, CapacityClass: CapacityClassOnDemand, Health: NodeHealthHealthy},
	}

	decision := Decide(workload, nodes)
	if decision.Outcome != OutcomePlaced {
		t.Fatalf("expected placed outcome, got %s (%s)", decision.Outcome, decision.Reason)
	}
	if decision.SelectedNode == nil || decision.SelectedNode.ID != "spare" {
		t.Fatalf("expected inference workload to prefer lower-utilization node, got %#v", decision.SelectedNode)
	}
}

func TestTrainingRejectsSpotCapacityEvenWhenTolerated(t *testing.T) {
	workload := Workload{
		ID:           "train-1",
		Type:         WorkloadTypeTraining,
		GPUType:      "A100",
		GPUCount:     1,
		Priority:     PriorityHigh,
		SubmittedAt:  time.Unix(300, 0),
		SpotTolerant: true,
	}

	nodes := []Node{
		{ID: "spot-only", GPUType: "A100", TotalGPUs: 4, AllocatedGPUs: 0, CapacityClass: CapacityClassSpot, Health: NodeHealthHealthy},
	}

	decision := Decide(workload, nodes)

	if decision.Outcome != OutcomeQueued {
		t.Fatalf("expected queued outcome, got %s", decision.Outcome)
	}
	if len(decision.RejectedNodes) != 1 {
		t.Fatalf("expected one rejected node, got %d", len(decision.RejectedNodes))
	}
	if got := decision.RejectedNodes[0].Reason; got == "" {
		t.Fatalf("expected rejection reason")
	}
}
