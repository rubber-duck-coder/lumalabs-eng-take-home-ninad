package domain

import "testing"

func TestNodeCapacityHelpers(t *testing.T) {
	node := Node{
		TotalGPUs:     8,
		AllocatedGPUs: 3,
		Health:        NodeHealthHealthy,
	}

	if got := node.FreeGPUs(); got != 5 {
		t.Fatalf("expected 5 free GPUs, got %d", got)
	}
	if !node.CanFit(4) {
		t.Fatalf("expected node to fit 4 GPUs")
	}
	if node.CanFit(6) {
		t.Fatalf("expected node not to fit 6 GPUs")
	}
}

func TestWorkloadStateHelpers(t *testing.T) {
	workload := Workload{State: WorkloadStateRunning}
	if !workload.IsActive() {
		t.Fatalf("expected running workload to be active")
	}
	if workload.IsTerminal() {
		t.Fatalf("expected running workload not to be terminal")
	}

	workload.State = WorkloadStatePreempted
	if !workload.IsTerminal() {
		t.Fatalf("expected preempted workload to be terminal")
	}
}
