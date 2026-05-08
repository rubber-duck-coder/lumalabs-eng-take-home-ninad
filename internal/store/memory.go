package store

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/scheduler"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid value")
)

type SchedulingResult struct {
	Workload domain.Workload    `json:"workload"`
	Decision scheduler.Decision `json:"decision"`
}

type DisruptionResult struct {
	Node              domain.Node        `json:"node"`
	AffectedWorkloads []domain.Workload  `json:"affected_workloads"`
	Scheduled         []SchedulingResult `json:"scheduled"`
}

type MemoryStore struct {
	mu        sync.RWMutex
	workloads map[string]domain.Workload
	nodes     map[string]domain.Node
	events    map[string]domain.Event
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		workloads: make(map[string]domain.Workload),
		nodes:     make(map[string]domain.Node),
		events:    make(map[string]domain.Event),
	}
}

func NewSeededMemoryStore() *MemoryStore {
	store := NewMemoryStore()
	_, _ = store.SeedDemoData()
	return store
}

func (s *MemoryStore) SeedDemoData() (DemoDataSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.resetLocked()
	seedTime := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)

	nodes := []domain.Node{
		{
			ID:                 "node-a100-od-1",
			GPUType:            "A100",
			TotalGPUs:          8,
			AllocatedGPUs:      4,
			Region:             "us-west-2",
			DataCenter:         "sfo-1",
			Zone:               "usw2-az1",
			Provider:           "aws",
			CapacityClass:      domain.CapacityClassOnDemand,
			Health:             domain.NodeHealthHealthy,
			RunningWorkloadIDs: []string{"workload-seed-train-1"},
			CreatedAt:          seedTime,
			UpdatedAt:          seedTime,
		},
		{
			ID:            "node-a100-spot-1",
			GPUType:       "A100",
			TotalGPUs:     8,
			AllocatedGPUs: 0,
			Region:        "us-west-2",
			DataCenter:    "sfo-1",
			Zone:          "usw2-az2",
			Provider:      "aws",
			CapacityClass: domain.CapacityClassSpot,
			Health:        domain.NodeHealthHealthy,
			CreatedAt:     seedTime.Add(1 * time.Minute),
			UpdatedAt:     seedTime.Add(1 * time.Minute),
		},
		{
			ID:            "node-h100-od-1",
			GPUType:       "H100",
			TotalGPUs:     16,
			AllocatedGPUs: 8,
			Region:        "us-east-1",
			DataCenter:    "iad-1",
			Zone:          "use1-az1",
			Provider:      "gcp",
			CapacityClass: domain.CapacityClassOnDemand,
			Health:        domain.NodeHealthHealthy,
			CreatedAt:     seedTime.Add(2 * time.Minute),
			UpdatedAt:     seedTime.Add(2 * time.Minute),
		},
		{
			ID:            "node-h100-spot-1",
			GPUType:       "H100",
			TotalGPUs:     16,
			AllocatedGPUs: 0,
			Region:        "us-east-1",
			DataCenter:    "iad-1",
			Zone:          "use1-az2",
			Provider:      "gcp",
			CapacityClass: domain.CapacityClassSpot,
			Health:        domain.NodeHealthHealthy,
			CreatedAt:     seedTime.Add(3 * time.Minute),
			UpdatedAt:     seedTime.Add(3 * time.Minute),
		},
		{
			ID:            "node-l4-od-1",
			GPUType:       "L4",
			TotalGPUs:     4,
			AllocatedGPUs: 1,
			Region:        "eu-west-1",
			DataCenter:    "dub-1",
			Zone:          "euw1-az1",
			Provider:      "azure",
			CapacityClass: domain.CapacityClassOnDemand,
			Health:        domain.NodeHealthRecovering,
			CreatedAt:     seedTime.Add(4 * time.Minute),
			UpdatedAt:     seedTime.Add(4 * time.Minute),
		},
		{
			ID:            "node-l4-spot-1",
			GPUType:       "L4",
			TotalGPUs:     4,
			AllocatedGPUs: 0,
			Region:        "eu-west-1",
			DataCenter:    "dub-1",
			Zone:          "euw1-az2",
			Provider:      "azure",
			CapacityClass: domain.CapacityClassSpot,
			Health:        domain.NodeHealthHealthy,
			CreatedAt:     seedTime.Add(5 * time.Minute),
			UpdatedAt:     seedTime.Add(5 * time.Minute),
		},
	}
	for _, node := range nodes {
		s.nodes[node.ID] = cloneNode(node)
	}

	workloads := []domain.Workload{
		{
			ID:                    "workload-seed-train-1",
			Type:                  domain.WorkloadTypeTraining,
			GPUType:               "A100",
			GPUCount:              4,
			Priority:              domain.WorkloadPriorityHigh,
			DurationSeconds:       1800,
			SpotTolerant:          true,
			State:                 domain.WorkloadStateRunning,
			Placement:             &domain.Placement{NodeID: "node-a100-od-1", Region: "us-west-2", DataCenter: "sfo-1", Zone: "usw2-az1", Provider: "aws"},
			SchedulingExplanation: "placed on seeded on-demand capacity",
			SubmittedAt:           seedTime.Add(-15 * time.Minute),
			UpdatedAt:             seedTime.Add(-10 * time.Minute),
		},
		{
			ID:                    "workload-seed-queue-1",
			Type:                  domain.WorkloadTypeBatch,
			GPUType:               "H100",
			GPUCount:              12,
			Priority:              domain.WorkloadPriorityNormal,
			DurationSeconds:       900,
			SpotTolerant:          false,
			State:                 domain.WorkloadStatePending,
			StatusReason:          "queued; waiting for a larger H100 fit",
			SchedulingExplanation: "queued; waiting for a larger H100 fit",
			SubmittedAt:           seedTime.Add(-8 * time.Minute),
			UpdatedAt:             seedTime.Add(-8 * time.Minute),
		},
		{
			ID:                    "workload-seed-spot-1",
			Type:                  domain.WorkloadTypeInference,
			GPUType:               "L4",
			GPUCount:              1,
			Priority:              domain.WorkloadPriorityLow,
			DurationSeconds:       300,
			SpotTolerant:          true,
			Resumable:             true,
			State:                 domain.WorkloadStatePending,
			StatusReason:          "queued for spot capacity",
			SchedulingExplanation: "queued for spot capacity",
			SubmittedAt:           seedTime.Add(-5 * time.Minute),
			UpdatedAt:             seedTime.Add(-5 * time.Minute),
		},
	}
	for _, workload := range workloads {
		s.workloads[workload.ID] = cloneWorkload(workload)
	}

	events := []domain.Event{
		{
			ID:        "event-seed-1",
			Timestamp: seedTime.Add(-4 * time.Minute),
			Type:      "seed.demo_data_loaded",
			Actor:     "system",
			Message:   "demo data loaded into memory store",
			Metadata: map[string]string{
				"nodes":     "6",
				"workloads": "3",
			},
		},
		{
			ID:         "event-seed-2",
			Timestamp:  seedTime.Add(-3 * time.Minute),
			Type:       "seed.demo_queue",
			Actor:      "system",
			WorkloadID: "workload-seed-queue-1",
			Message:    "seeded queued workload for dashboard review",
		},
	}
	for _, event := range events {
		s.events[event.ID] = cloneEvent(event)
	}

	return DemoDataSummary{Nodes: len(nodes), Workloads: len(workloads), Events: len(events)}, nil
}

func (s *MemoryStore) Clear() (DemoDataSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary := DemoDataSummary{
		Nodes:     len(s.nodes),
		Workloads: len(s.workloads),
		Events:    len(s.events),
	}
	s.resetLocked()
	return summary, nil
}

func (s *MemoryStore) resetLocked() {
	s.workloads = make(map[string]domain.Workload)
	s.nodes = make(map[string]domain.Node)
	s.events = make(map[string]domain.Event)
}

func (s *MemoryStore) CreateWorkload(workload domain.Workload) (domain.Workload, error) {
	if workload.ID == "" {
		return domain.Workload{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workloads[workload.ID]; exists {
		return domain.Workload{}, ErrConflict
	}

	s.workloads[workload.ID] = cloneWorkload(workload)
	return cloneWorkload(workload), nil
}

func (s *MemoryStore) GetWorkload(id string) (domain.Workload, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workload, exists := s.workloads[id]
	if !exists {
		return domain.Workload{}, false
	}
	return cloneWorkload(workload), true
}

func (s *MemoryStore) ListWorkloads() []domain.Workload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workloads := make([]domain.Workload, 0, len(s.workloads))
	for _, workload := range s.workloads {
		workloads = append(workloads, cloneWorkload(workload))
	}
	sort.Slice(workloads, func(i, j int) bool {
		if workloads[i].SubmittedAt.Equal(workloads[j].SubmittedAt) {
			return workloads[i].ID < workloads[j].ID
		}
		return workloads[i].SubmittedAt.Before(workloads[j].SubmittedAt)
	})
	return workloads
}

func (s *MemoryStore) UpdateWorkload(id string, fn func(*domain.Workload) error) (domain.Workload, error) {
	if fn == nil {
		return domain.Workload{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	workload, exists := s.workloads[id]
	if !exists {
		return domain.Workload{}, ErrNotFound
	}

	updated := cloneWorkload(workload)
	if err := fn(&updated); err != nil {
		return domain.Workload{}, err
	}

	s.workloads[id] = cloneWorkload(updated)
	return cloneWorkload(updated), nil
}

func (s *MemoryStore) ScheduleWorkload(id string, now time.Time) (SchedulingResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.scheduleWorkloadLocked(id, now)
}

func (s *MemoryStore) SchedulePendingWorkloads(now time.Time) ([]SchedulingResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.schedulePendingLocked(now)
}

func (s *MemoryStore) FailNode(id string, now time.Time) (DisruptionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, exists := s.nodes[id]
	if !exists {
		return DisruptionResult{}, ErrNotFound
	}

	affected := s.evictNodeWorkloadsLocked(&node, now, domain.WorkloadStatePending, "node failed; workload re-queued")
	node.Health = domain.NodeHealthFailed
	node.UpdatedAt = now
	s.nodes[id] = cloneNode(node)

	scheduled, err := s.schedulePendingLocked(now)
	if err != nil {
		return DisruptionResult{}, err
	}
	return DisruptionResult{
		Node:              cloneNode(node),
		AffectedWorkloads: cloneWorkloads(affected),
		Scheduled:         cloneSchedulingResults(scheduled),
	}, nil
}

func (s *MemoryStore) RecoverNode(id string, now time.Time) (DisruptionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, exists := s.nodes[id]
	if !exists {
		return DisruptionResult{}, ErrNotFound
	}

	node.Health = domain.NodeHealthHealthy
	node.UpdatedAt = now
	s.nodes[id] = cloneNode(node)

	scheduled, err := s.schedulePendingLocked(now)
	if err != nil {
		return DisruptionResult{}, err
	}
	return DisruptionResult{
		Node:              cloneNode(node),
		AffectedWorkloads: nil,
		Scheduled:         cloneSchedulingResults(scheduled),
	}, nil
}

func (s *MemoryStore) PreemptSpotNode(id string, now time.Time) (DisruptionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, exists := s.nodes[id]
	if !exists {
		return DisruptionResult{}, ErrNotFound
	}
	if node.CapacityClass != domain.CapacityClassSpot {
		return DisruptionResult{}, ErrInvalid
	}

	affected := s.evictSpotNodeWorkloadsLocked(&node, now, "spot node preempted; workload re-queued")
	node.Health = domain.NodeHealthFailed
	node.UpdatedAt = now
	s.nodes[id] = cloneNode(node)

	scheduled, err := s.schedulePendingLocked(now)
	if err != nil {
		return DisruptionResult{}, err
	}
	return DisruptionResult{
		Node:              cloneNode(node),
		AffectedWorkloads: cloneWorkloads(affected),
		Scheduled:         cloneSchedulingResults(scheduled),
	}, nil
}

func (s *MemoryStore) scheduleWorkloadLocked(id string, now time.Time) (SchedulingResult, error) {

	workload, exists := s.workloads[id]
	if !exists {
		return SchedulingResult{}, ErrNotFound
	}
	if workload.State != domain.WorkloadStatePending {
		return SchedulingResult{Workload: cloneWorkload(workload)}, nil
	}

	decision := scheduler.Decide(toSchedulerWorkload(workload), toSchedulerNodesLocked(s.nodes))
	updated := cloneWorkload(workload)
	updated.UpdatedAt = now
	updated.StatusReason = decision.Reason
	updated.SchedulingExplanation = decision.Reason

	if decision.Outcome == scheduler.OutcomeQueued {
		if preemptedResult, ok, err := s.tryPriorityPreemptionLocked(workload, now, decision); err != nil {
			return SchedulingResult{}, err
		} else if ok {
			return preemptedResult, nil
		}
		updated.State = domain.WorkloadStatePending
		updated.Placement = nil
		s.workloads[id] = cloneWorkload(updated)
		return SchedulingResult{Workload: cloneWorkload(updated), Decision: decision}, nil
	}

	if decision.SelectedNode == nil {
		return SchedulingResult{}, ErrInvalid
	}

	node, exists := s.nodes[decision.SelectedNode.ID]
	if !exists {
		return SchedulingResult{}, ErrNotFound
	}
	if node.FreeGPUs() < workload.GPUCount {
		return SchedulingResult{}, fmt.Errorf("%w: insufficient node capacity", ErrConflict)
	}

	node.AllocatedGPUs += workload.GPUCount
	if node.AllocatedGPUs > node.TotalGPUs {
		return SchedulingResult{}, fmt.Errorf("%w: node over allocation", ErrConflict)
	}
	node.RunningWorkloadIDs = append(node.RunningWorkloadIDs, workload.ID)
	node.UpdatedAt = now

	updated.State = domain.WorkloadStateRunning
	updated.Placement = &domain.Placement{
		NodeID:     node.ID,
		Region:     node.Region,
		DataCenter: node.DataCenter,
		Zone:       node.Zone,
		Provider:   node.Provider,
	}

	s.nodes[node.ID] = cloneNode(node)
	s.workloads[id] = cloneWorkload(updated)
	return SchedulingResult{Workload: cloneWorkload(updated), Decision: decision}, nil
}

type preemptionCandidate struct {
	node          domain.Node
	victims       []domain.Workload
	reclaimedGPUs int
	score         int
}

func (s *MemoryStore) tryPriorityPreemptionLocked(workload domain.Workload, now time.Time, originalDecision scheduler.Decision) (SchedulingResult, bool, error) {
	if priorityRank(workload.Priority) <= priorityRank(domain.WorkloadPriorityLow) {
		return SchedulingResult{}, false, nil
	}

	candidates := s.priorityPreemptionCandidatesLocked(workload)
	if len(candidates) == 0 {
		return SchedulingResult{}, false, nil
	}

	selected := candidates[0]
	node := s.nodes[selected.node.ID]
	beforePlacement := toSchedulerNode(selected.node)

	reason := fmt.Sprintf("preempted %d lower-priority workload(s) on node %s", len(selected.victims), node.ID)
	affected := s.preemptWorkloadsOnNodeLocked(&node, selected.victims, now, reason)
	if len(affected) != len(selected.victims) {
		return SchedulingResult{}, false, ErrConflict
	}

	node.AllocatedGPUs += workload.GPUCount
	if node.AllocatedGPUs > node.TotalGPUs {
		return SchedulingResult{}, false, fmt.Errorf("%w: node over allocation", ErrConflict)
	}
	node.RunningWorkloadIDs = append(node.RunningWorkloadIDs, workload.ID)
	node.UpdatedAt = now

	updated := cloneWorkload(workload)
	updated.State = domain.WorkloadStateRunning
	updated.UpdatedAt = now
	updated.StatusReason = reason
	updated.SchedulingExplanation = reason
	updated.Placement = &domain.Placement{
		NodeID:     node.ID,
		Region:     node.Region,
		DataCenter: node.DataCenter,
		Zone:       node.Zone,
		Provider:   node.Provider,
	}

	s.nodes[node.ID] = cloneNode(node)
	s.workloads[workload.ID] = cloneWorkload(updated)

	decision := scheduler.Decision{
		Outcome:       scheduler.OutcomePlaced,
		SelectedNode:  &beforePlacement,
		Reason:        reason,
		RejectedNodes: originalDecision.RejectedNodes,
	}
	return SchedulingResult{Workload: cloneWorkload(updated), Decision: decision}, true, nil
}

func (s *MemoryStore) priorityPreemptionCandidatesLocked(workload domain.Workload) []preemptionCandidate {
	candidates := make([]preemptionCandidate, 0)
	for _, node := range s.nodes {
		candidate, ok := s.buildPriorityPreemptionCandidateLocked(workload, node)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if len(candidates[i].victims) != len(candidates[j].victims) {
			return len(candidates[i].victims) < len(candidates[j].victims)
		}
		if candidates[i].reclaimedGPUs != candidates[j].reclaimedGPUs {
			return candidates[i].reclaimedGPUs < candidates[j].reclaimedGPUs
		}
		if candidates[i].score != candidates[j].score {
			return candidates[i].score < candidates[j].score
		}
		return candidates[i].node.ID < candidates[j].node.ID
	})
	return candidates
}

func (s *MemoryStore) buildPriorityPreemptionCandidateLocked(workload domain.Workload, node domain.Node) (preemptionCandidate, bool) {
	if node.Health != domain.NodeHealthHealthy {
		return preemptionCandidate{}, false
	}
	if node.GPUType != workload.GPUType {
		return preemptionCandidate{}, false
	}
	if !prioritySpotCompatible(workload, node) {
		return preemptionCandidate{}, false
	}

	victims := s.selectPreemptableVictimsLocked(node.ID, workload.Priority, workload.GPUCount)
	if len(victims) == 0 {
		return preemptionCandidate{}, false
	}

	reclaimed := 0
	for _, victim := range victims {
		reclaimed += victim.GPUCount
	}
	if node.FreeGPUs()+reclaimed < workload.GPUCount {
		return preemptionCandidate{}, false
	}

	candidateNode := cloneNode(node)
	candidateNode.AllocatedGPUs = max(candidateNode.AllocatedGPUs-reclaimed, 0)
	candidateNode.RunningWorkloadIDs = removeWorkloadIDs(candidateNode.RunningWorkloadIDs, victims)

	return preemptionCandidate{
		node:          candidateNode,
		victims:       victims,
		reclaimedGPUs: reclaimed,
		score:         priorityPlacementScore(workload, candidateNode),
	}, true
}

func (s *MemoryStore) selectPreemptableVictimsLocked(nodeID string, priority domain.WorkloadPriority, gpuCount int) []domain.Workload {
	running := make([]domain.Workload, 0)
	for _, workload := range s.workloads {
		if workload.State != domain.WorkloadStateRunning {
			continue
		}
		if workload.Placement == nil || workload.Placement.NodeID != nodeID {
			continue
		}
		if priorityRank(workload.Priority) >= priorityRank(priority) {
			continue
		}
		running = append(running, cloneWorkload(workload))
	}

	sort.SliceStable(running, func(i, j int) bool {
		if priorityRank(running[i].Priority) != priorityRank(running[j].Priority) {
			return priorityRank(running[i].Priority) < priorityRank(running[j].Priority)
		}
		if running[i].GPUCount != running[j].GPUCount {
			return running[i].GPUCount > running[j].GPUCount
		}
		if !running[i].SubmittedAt.Equal(running[j].SubmittedAt) {
			return running[i].SubmittedAt.After(running[j].SubmittedAt)
		}
		return running[i].ID < running[j].ID
	})

	reclaimed := 0
	victims := make([]domain.Workload, 0)
	for _, workload := range running {
		victims = append(victims, workload)
		reclaimed += workload.GPUCount
		if reclaimed >= gpuCount {
			break
		}
	}

	return victims
}

func (s *MemoryStore) schedulePendingLocked(now time.Time) ([]SchedulingResult, error) {
	pending := make([]scheduler.Workload, 0)
	for _, workload := range s.workloads {
		if workload.State == domain.WorkloadStatePending {
			pending = append(pending, toSchedulerWorkload(workload))
		}
	}
	scheduler.OrderPendingWorkloads(pending)

	results := make([]SchedulingResult, 0, len(pending))
	for _, workload := range pending {
		result, err := s.scheduleWorkloadLocked(workload.ID, now)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *MemoryStore) CreateNode(node domain.Node) (domain.Node, error) {
	if node.ID == "" {
		return domain.Node{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nodes[node.ID]; exists {
		return domain.Node{}, ErrConflict
	}

	s.nodes[node.ID] = cloneNode(node)
	return cloneNode(node), nil
}

func (s *MemoryStore) GetNode(id string) (domain.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.nodes[id]
	if !exists {
		return domain.Node{}, false
	}
	return cloneNode(node), true
}

func (s *MemoryStore) ListNodes() []domain.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]domain.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, cloneNode(node))
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

func (s *MemoryStore) UpdateNode(id string, fn func(*domain.Node) error) (domain.Node, error) {
	if fn == nil {
		return domain.Node{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node, exists := s.nodes[id]
	if !exists {
		return domain.Node{}, ErrNotFound
	}

	updated := cloneNode(node)
	if err := fn(&updated); err != nil {
		return domain.Node{}, err
	}

	s.nodes[id] = cloneNode(updated)
	return cloneNode(updated), nil
}

func (s *MemoryStore) CreateEvent(event domain.Event) (domain.Event, error) {
	if event.ID == "" {
		return domain.Event{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.events[event.ID]; exists {
		return domain.Event{}, ErrConflict
	}

	s.events[event.ID] = cloneEvent(event)
	return cloneEvent(event), nil
}

func (s *MemoryStore) GetEvent(id string) (domain.Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	event, exists := s.events[id]
	if !exists {
		return domain.Event{}, false
	}
	return cloneEvent(event), true
}

func (s *MemoryStore) ListEvents() []domain.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := make([]domain.Event, 0, len(s.events))
	for _, event := range s.events {
		events = append(events, cloneEvent(event))
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events
}

func (s *MemoryStore) UpdateEvent(id string, fn func(*domain.Event) error) (domain.Event, error) {
	if fn == nil {
		return domain.Event{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	event, exists := s.events[id]
	if !exists {
		return domain.Event{}, ErrNotFound
	}

	updated := cloneEvent(event)
	if err := fn(&updated); err != nil {
		return domain.Event{}, err
	}

	s.events[id] = cloneEvent(updated)
	return cloneEvent(updated), nil
}

func cloneWorkload(workload domain.Workload) domain.Workload {
	if workload.Placement != nil {
		placement := *workload.Placement
		workload.Placement = &placement
	}
	return workload
}

func cloneNode(node domain.Node) domain.Node {
	node.RunningWorkloadIDs = append([]string(nil), node.RunningWorkloadIDs...)
	return node
}

func cloneEvent(event domain.Event) domain.Event {
	if event.Metadata != nil {
		metadata := make(map[string]string, len(event.Metadata))
		for key, value := range event.Metadata {
			metadata[key] = value
		}
		event.Metadata = metadata
	}
	return event
}

func cloneWorkloads(workloads []domain.Workload) []domain.Workload {
	out := make([]domain.Workload, 0, len(workloads))
	for _, workload := range workloads {
		out = append(out, cloneWorkload(workload))
	}
	return out
}

func cloneSchedulingResults(results []SchedulingResult) []SchedulingResult {
	out := make([]SchedulingResult, 0, len(results))
	for _, result := range results {
		out = append(out, SchedulingResult{
			Workload: cloneWorkload(result.Workload),
			Decision: result.Decision,
		})
	}
	return out
}

func (s *MemoryStore) preemptWorkloadsOnNodeLocked(node *domain.Node, victims []domain.Workload, now time.Time, reason string) []domain.Workload {
	affected := make([]domain.Workload, 0, len(victims))
	victimSet := make(map[string]struct{}, len(victims))
	reclaimed := 0
	for _, victim := range victims {
		victimSet[victim.ID] = struct{}{}
		reclaimed += victim.GPUCount
	}

	for workloadID, workload := range s.workloads {
		if workload.State != domain.WorkloadStateRunning {
			continue
		}
		if workload.Placement == nil || workload.Placement.NodeID != node.ID {
			continue
		}
		if _, ok := victimSet[workloadID]; !ok {
			continue
		}

		updated := cloneWorkload(workload)
		updated.State = domain.WorkloadStatePending
		updated.Placement = nil
		updated.StatusReason = reason
		updated.SchedulingExplanation = reason
		updated.PreemptNoticeSeconds = 0
		updated.DrainStartedAt = timePtr(now)
		updated.CheckpointState = "drained"
		updated.ResumeEligible = workload.Resumable
		if workload.Resumable {
			updated.CheckpointState = "checkpointed"
		}
		updated.UpdatedAt = now
		s.workloads[workloadID] = cloneWorkload(updated)
		affected = append(affected, cloneWorkload(updated))
	}

	if node.AllocatedGPUs < reclaimed {
		node.AllocatedGPUs = 0
	} else {
		node.AllocatedGPUs -= reclaimed
	}
	node.RunningWorkloadIDs = removeWorkloadIDs(node.RunningWorkloadIDs, victims)
	return affected
}

func (s *MemoryStore) evictNodeWorkloadsLocked(node *domain.Node, now time.Time, state domain.WorkloadState, reason string) []domain.Workload {
	affected := make([]domain.Workload, 0)
	for workloadID, workload := range s.workloads {
		if workload.State != domain.WorkloadStateRunning {
			continue
		}
		if workload.Placement == nil || workload.Placement.NodeID != node.ID {
			continue
		}

		updated := cloneWorkload(workload)
		updated.State = state
		updated.Placement = nil
		updated.StatusReason = reason
		updated.SchedulingExplanation = reason
		updated.PreemptNoticeSeconds = 0
		updated.DrainStartedAt = timePtr(now)
		updated.CheckpointState = "drained"
		updated.ResumeEligible = false
		updated.UpdatedAt = now
		s.workloads[workloadID] = cloneWorkload(updated)
		affected = append(affected, cloneWorkload(updated))
	}

	node.AllocatedGPUs = 0
	node.RunningWorkloadIDs = nil
	return affected
}

func (s *MemoryStore) evictSpotNodeWorkloadsLocked(node *domain.Node, now time.Time, reason string) []domain.Workload {
	affected := s.evictNodeWorkloadsLocked(node, now, domain.WorkloadStatePending, reason)
	for _, workload := range affected {
		updated := workload
		updated.PreemptNoticeSeconds = 30
		updated.DrainStartedAt = timePtr(now)
		updated.CheckpointState = "checkpointed"
		updated.ResumeEligible = workload.Resumable
		if !workload.Resumable {
			updated.CheckpointState = "drained"
		}
		s.workloads[workload.ID] = cloneWorkload(updated)
	}
	return s.refreshAffectedWorkloadsLocked(affected)
}

func (s *MemoryStore) refreshAffectedWorkloadsLocked(workloads []domain.Workload) []domain.Workload {
	refreshed := make([]domain.Workload, 0, len(workloads))
	for _, workload := range workloads {
		if current, ok := s.workloads[workload.ID]; ok {
			refreshed = append(refreshed, cloneWorkload(current))
		}
	}
	return refreshed
}

func timePtr(t time.Time) *time.Time {
	copied := t
	return &copied
}

func toSchedulerWorkload(workload domain.Workload) scheduler.Workload {
	return scheduler.Workload{
		ID:           workload.ID,
		Type:         scheduler.WorkloadType(workload.Type),
		GPUType:      workload.GPUType,
		GPUCount:     workload.GPUCount,
		Priority:     toSchedulerPriority(workload.Priority),
		SubmittedAt:  workload.SubmittedAt,
		SpotTolerant: workload.SpotTolerant,
	}
}

func toSchedulerPriority(priority domain.WorkloadPriority) scheduler.Priority {
	switch priority {
	case domain.WorkloadPriorityHigh:
		return scheduler.PriorityHigh
	case domain.WorkloadPriorityNormal:
		return scheduler.PriorityNormal
	default:
		return scheduler.PriorityLow
	}
}

func priorityRank(priority domain.WorkloadPriority) int {
	switch priority {
	case domain.WorkloadPriorityHigh:
		return 3
	case domain.WorkloadPriorityNormal:
		return 2
	default:
		return 1
	}
}

func toSchedulerNodesLocked(nodes map[string]domain.Node) []scheduler.Node {
	out := make([]scheduler.Node, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, toSchedulerNode(node))
	}
	return out
}

func toSchedulerNode(node domain.Node) scheduler.Node {
	return scheduler.Node{
		ID:            node.ID,
		GPUType:       node.GPUType,
		TotalGPUs:     node.TotalGPUs,
		AllocatedGPUs: node.AllocatedGPUs,
		CapacityClass: scheduler.CapacityClass(node.CapacityClass),
		Health:        scheduler.NodeHealth(node.Health),
		Region:        node.Region,
		Zone:          node.Zone,
		Provider:      node.Provider,
	}
}

func prioritySpotCompatible(workload domain.Workload, node domain.Node) bool {
	switch workload.Type {
	case domain.WorkloadTypeTraining:
		return node.CapacityClass == domain.CapacityClassOnDemand
	case domain.WorkloadTypeBatch:
		if workload.SpotTolerant {
			return true
		}
		return node.CapacityClass == domain.CapacityClassOnDemand
	case domain.WorkloadTypeInference:
		if node.CapacityClass == domain.CapacityClassOnDemand {
			return true
		}
		return workload.SpotTolerant && node.CapacityClass == domain.CapacityClassSpot
	default:
		return false
	}
}

func priorityPlacementScore(workload domain.Workload, node domain.Node) int {
	switch workload.Type {
	case domain.WorkloadTypeTraining:
		score := 0
		if node.CapacityClass != domain.CapacityClassOnDemand {
			score += 1000
		}
		score += max(node.FreeGPUs()-workload.GPUCount, 0)
		return score
	case domain.WorkloadTypeBatch:
		if workload.SpotTolerant {
			score := 0
			if node.CapacityClass != domain.CapacityClassSpot {
				score += 1000
			}
			score += max(node.FreeGPUs()-workload.GPUCount, 0)
			return score
		}
		score := 0
		if node.CapacityClass != domain.CapacityClassOnDemand {
			score += 1000
		}
		score += max(node.FreeGPUs()-workload.GPUCount, 0)
		return score
	case domain.WorkloadTypeInference:
		score := 0
		if node.CapacityClass == domain.CapacityClassOnDemand {
			score += 0
		} else {
			score += 250
		}
		score += priorityUtilizationPenalty(node)
		return score
	default:
		return 1
	}
}

func priorityUtilizationPenalty(node domain.Node) int {
	if node.TotalGPUs <= 0 {
		return 1000
	}
	return (node.AllocatedGPUs * 100) / node.TotalGPUs
}

func removeWorkloadIDs(existing []string, victims []domain.Workload) []string {
	if len(existing) == 0 || len(victims) == 0 {
		return append([]string(nil), existing...)
	}
	toRemove := make(map[string]struct{}, len(victims))
	for _, victim := range victims {
		toRemove[victim.ID] = struct{}{}
	}
	out := make([]string, 0, len(existing))
	for _, id := range existing {
		if _, ok := toRemove[id]; ok {
			continue
		}
		out = append(out, id)
	}
	return out
}
