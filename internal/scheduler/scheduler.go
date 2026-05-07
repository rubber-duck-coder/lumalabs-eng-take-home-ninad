package scheduler

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type WorkloadType string

const (
	WorkloadTypeTraining  WorkloadType = "training"
	WorkloadTypeInference WorkloadType = "inference"
	WorkloadTypeBatch     WorkloadType = "batch"
)

type Priority int

const (
	PriorityLow Priority = iota + 1
	PriorityNormal
	PriorityHigh
)

type CapacityClass string

const (
	CapacityClassOnDemand CapacityClass = "on_demand"
	CapacityClassSpot     CapacityClass = "spot"
)

type NodeHealth string

const (
	NodeHealthHealthy    NodeHealth = "healthy"
	NodeHealthFailed     NodeHealth = "failed"
	NodeHealthRecovering NodeHealth = "recovering"
)

type Workload struct {
	ID           string
	Type         WorkloadType
	GPUType      string
	GPUCount     int
	Priority     Priority
	SubmittedAt  time.Time
	SpotTolerant bool
}

type Node struct {
	ID            string
	GPUType       string
	TotalGPUs     int
	AllocatedGPUs int
	CapacityClass CapacityClass
	Health        NodeHealth
}

func (n Node) FreeGPUs() int {
	return n.TotalGPUs - n.AllocatedGPUs
}

type Outcome string

const (
	OutcomePlaced Outcome = "placed"
	OutcomeQueued Outcome = "queued"
)

type RejectedNode struct {
	NodeID string
	Reason string
}

type Decision struct {
	Outcome       Outcome
	SelectedNode  *Node
	Reason        string
	RejectedNodes []RejectedNode
}

func OrderPendingWorkloads(workloads []Workload) {
	sort.SliceStable(workloads, func(i, j int) bool {
		return WorkloadLess(workloads[i], workloads[j])
	})
}

func WorkloadLess(a, b Workload) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if !a.SubmittedAt.Equal(b.SubmittedAt) {
		return a.SubmittedAt.Before(b.SubmittedAt)
	}
	return a.ID < b.ID
}

func Decide(workload Workload, nodes []Node) Decision {
	rejected := make([]RejectedNode, 0, len(nodes))
	candidates := make([]candidateNode, 0, len(nodes))

	for _, node := range nodes {
		reason, eligible := eligibilityReason(workload, node)
		if !eligible {
			rejected = append(rejected, RejectedNode{NodeID: node.ID, Reason: reason})
			continue
		}

		candidates = append(candidates, candidateNode{
			node:       node,
			classRank:  capacityPreference(workload, node),
			surplusGPU: node.FreeGPUs() - workload.GPUCount,
		})
	}

	if len(candidates) == 0 {
		return Decision{
			Outcome:       OutcomeQueued,
			Reason:        queueReason(workload, rejected),
			RejectedNodes: rejected,
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].classRank != candidates[j].classRank {
			return candidates[i].classRank < candidates[j].classRank
		}
		if candidates[i].surplusGPU != candidates[j].surplusGPU {
			return candidates[i].surplusGPU < candidates[j].surplusGPU
		}
		return candidates[i].node.ID < candidates[j].node.ID
	})

	selected := candidates[0].node
	return Decision{
		Outcome:       OutcomePlaced,
		SelectedNode:  &selected,
		Reason:        placementReason(workload, selected),
		RejectedNodes: append(rejected, rejectedFromCandidates(candidates[1:])...),
	}
}

type candidateNode struct {
	node       Node
	classRank  int
	surplusGPU int
}

func rejectedFromCandidates(candidates []candidateNode) []RejectedNode {
	out := make([]RejectedNode, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, RejectedNode{
			NodeID: c.node.ID,
			Reason: fmt.Sprintf("node %s was not selected after deterministic tie-breaks", c.node.ID),
		})
	}
	return out
}

func eligibilityReason(workload Workload, node Node) (string, bool) {
	var reasons []string

	if node.Health != NodeHealthHealthy {
		reasons = append(reasons, "node is not healthy")
	}
	if node.GPUType != workload.GPUType {
		reasons = append(reasons, fmt.Sprintf("gpu type mismatch: want %s, got %s", workload.GPUType, node.GPUType))
	}
	if node.FreeGPUs() < workload.GPUCount {
		reasons = append(reasons, fmt.Sprintf("insufficient free gpus: need %d, have %d", workload.GPUCount, node.FreeGPUs()))
	}
	if !spotCompatible(workload, node) {
		reasons = append(reasons, "capacity class is not compatible with workload policy")
	}

	if len(reasons) > 0 {
		return strings.Join(reasons, "; "), false
	}
	return "", true
}

func spotCompatible(workload Workload, node Node) bool {
	switch workload.Type {
	case WorkloadTypeTraining:
		return node.CapacityClass == CapacityClassOnDemand
	case WorkloadTypeBatch:
		if workload.SpotTolerant {
			return true
		}
		return node.CapacityClass == CapacityClassOnDemand
	case WorkloadTypeInference:
		if node.CapacityClass == CapacityClassOnDemand {
			return true
		}
		return workload.SpotTolerant && node.CapacityClass == CapacityClassSpot
	default:
		return false
	}
}

func capacityPreference(workload Workload, node Node) int {
	switch workload.Type {
	case WorkloadTypeTraining:
		return 0
	case WorkloadTypeBatch:
		if workload.SpotTolerant {
			if node.CapacityClass == CapacityClassSpot {
				return 0
			}
			return 1
		}
		return 0
	case WorkloadTypeInference:
		if node.CapacityClass == CapacityClassOnDemand {
			return 0
		}
		return 1
	default:
		return 1
	}
}

func placementReason(workload Workload, node Node) string {
	switch workload.Type {
	case WorkloadTypeTraining:
		return fmt.Sprintf("placed on healthy on-demand %s node %s with %d free gpus", node.GPUType, node.ID, node.FreeGPUs())
	case WorkloadTypeBatch:
		if workload.SpotTolerant && node.CapacityClass == CapacityClassSpot {
			return fmt.Sprintf("batch workload prefers spot and node %s is spot capacity", node.ID)
		}
		return fmt.Sprintf("batch workload placed on node %s", node.ID)
	case WorkloadTypeInference:
		if node.CapacityClass == CapacityClassOnDemand {
			return fmt.Sprintf("inference workload placed on on-demand node %s", node.ID)
		}
		return fmt.Sprintf("inference workload placed on tolerated spot node %s", node.ID)
	default:
		return fmt.Sprintf("placed on node %s", node.ID)
	}
}

func queueReason(workload Workload, rejected []RejectedNode) string {
	if len(rejected) == 0 {
		return "no candidate nodes available"
	}

	var parts []string
	for _, node := range rejected {
		parts = append(parts, node.Reason)
	}

	switch workload.Type {
	case WorkloadTypeTraining:
		return fmt.Sprintf("no healthy on-demand %s node with %d free gpus; rejected: %s", workload.GPUType, workload.GPUCount, strings.Join(parts, " | "))
	case WorkloadTypeBatch:
		if workload.SpotTolerant {
			return fmt.Sprintf("no healthy %s node with %d free gpus; rejected: %s", workload.GPUType, workload.GPUCount, strings.Join(parts, " | "))
		}
		return fmt.Sprintf("no healthy on-demand %s node with %d free gpus; rejected: %s", workload.GPUType, workload.GPUCount, strings.Join(parts, " | "))
	case WorkloadTypeInference:
		if workload.SpotTolerant {
			return fmt.Sprintf("no healthy on-demand or tolerated spot %s node with %d free gpus; rejected: %s", workload.GPUType, workload.GPUCount, strings.Join(parts, " | "))
		}
		return fmt.Sprintf("no healthy on-demand %s node with %d free gpus; rejected: %s", workload.GPUType, workload.GPUCount, strings.Join(parts, " | "))
	default:
		return fmt.Sprintf("no eligible nodes for workload %s; rejected: %s", workload.ID, strings.Join(parts, " | "))
	}
}
