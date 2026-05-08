package controlplane

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/scheduler"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

type Service struct {
	store store.Store
	now   func() time.Time
	seq   atomic.Uint64
}

type SubmitWorkloadRequest struct {
	Type            domain.WorkloadType
	GPUType         string
	GPUCount        int
	Priority        domain.WorkloadPriority
	DurationSeconds int
	SpotTolerant    bool
}

type FleetSummary struct {
	TotalGPUs        int                          `json:"total_gpus"`
	AllocatedGPUs    int                          `json:"allocated_gpus"`
	AvailableGPUs    int                          `json:"available_gpus"`
	Utilization      float64                      `json:"utilization_percent"`
	GPUTypes         map[string]map[string]int    `json:"gpu_types"`
	WorkloadsByState map[domain.WorkloadState]int `json:"workloads_by_state"`
}

func New(appStore store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: appStore, now: now}
}

func (s *Service) SubmitWorkload(req SubmitWorkloadRequest) (domain.Workload, error) {
	now := s.now().UTC()
	workload := domain.Workload{
		ID:              s.nextID("workload", now),
		Type:            req.Type,
		GPUType:         strings.ToUpper(req.GPUType),
		GPUCount:        req.GPUCount,
		Priority:        req.Priority,
		DurationSeconds: req.DurationSeconds,
		SpotTolerant:    req.SpotTolerant,
		State:           domain.WorkloadStatePending,
		SubmittedAt:     now,
		UpdatedAt:       now,
	}

	created, err := s.store.CreateWorkload(workload)
	if err != nil {
		return domain.Workload{}, err
	}

	s.recordEvent("workload_submitted", "system", created.ID, "", "workload submitted", nil)
	return s.scheduleWorkload(created.ID), nil
}

func (s *Service) GetWorkload(id string) (domain.Workload, bool) {
	return s.store.GetWorkload(id)
}

func (s *Service) ListWorkloads() []domain.Workload {
	return s.store.ListWorkloads()
}

func (s *Service) ListNodes() []domain.Node {
	return s.store.ListNodes()
}

func (s *Service) ListEvents() []domain.Event {
	return s.store.ListEvents()
}

func (s *Service) FleetSummary() FleetSummary {
	nodes := s.store.ListNodes()
	workloads := s.store.ListWorkloads()

	var total, allocated int
	byGPU := map[string]map[string]int{}
	for _, node := range nodes {
		total += node.TotalGPUs
		allocated += node.AllocatedGPUs
		if _, ok := byGPU[node.GPUType]; !ok {
			byGPU[node.GPUType] = map[string]int{"total": 0, "allocated": 0}
		}
		byGPU[node.GPUType]["total"] += node.TotalGPUs
		byGPU[node.GPUType]["allocated"] += node.AllocatedGPUs
	}

	byState := map[domain.WorkloadState]int{}
	for _, workload := range workloads {
		byState[workload.State]++
	}

	utilization := 0.0
	if total > 0 {
		utilization = float64(allocated) / float64(total) * 100
	}

	return FleetSummary{
		TotalGPUs:        total,
		AllocatedGPUs:    allocated,
		AvailableGPUs:    total - allocated,
		Utilization:      utilization,
		GPUTypes:         byGPU,
		WorkloadsByState: byState,
	}
}

func (s *Service) SchedulerTick() ([]store.SchedulingResult, error) {
	results, err := s.store.SchedulePendingWorkloads(s.now().UTC())
	if err != nil {
		return nil, err
	}
	s.recordEvent("scheduler_tick", "scheduler", "", "", "scheduler tick completed", nil)
	s.recordScheduledResults(results)
	return results, nil
}

func (s *Service) SeedDemoData() (store.DemoDataSummary, error) {
	return s.store.SeedDemoData()
}

func (s *Service) ClearDemoData() (store.DemoDataSummary, error) {
	return s.store.Clear()
}

func (s *Service) FailNode(id string) (store.DisruptionResult, error) {
	result, err := s.store.FailNode(id, s.now().UTC())
	if err != nil {
		return store.DisruptionResult{}, err
	}
	s.recordEvent("node_failed", "admin", "", result.Node.ID, "node marked failed", nil)
	s.recordAffectedWorkloads("workload_disrupted", result.AffectedWorkloads, result.Node.ID)
	s.recordScheduledResults(result.Scheduled)
	return result, nil
}

func (s *Service) RecoverNode(id string) (store.DisruptionResult, error) {
	result, err := s.store.RecoverNode(id, s.now().UTC())
	if err != nil {
		return store.DisruptionResult{}, err
	}
	s.recordEvent("node_recovered", "admin", "", result.Node.ID, "node recovered", nil)
	s.recordScheduledResults(result.Scheduled)
	return result, nil
}

func (s *Service) PreemptSpotNode(id string) (store.DisruptionResult, error) {
	result, err := s.store.PreemptSpotNode(id, s.now().UTC())
	if err != nil {
		return store.DisruptionResult{}, err
	}
	s.recordEvent("node_spot_preempted", "admin", "", result.Node.ID, "spot node preempted", nil)
	s.recordAffectedWorkloads("workload_preempted", result.AffectedWorkloads, result.Node.ID)
	s.recordScheduledResults(result.Scheduled)
	return result, nil
}

func (s *Service) scheduleWorkload(id string) domain.Workload {
	workload, ok := s.store.GetWorkload(id)
	if !ok {
		return domain.Workload{}
	}

	now := s.now().UTC()
	result, err := s.store.ScheduleWorkload(id, now)
	if err != nil {
		return workload
	}

	s.recordSchedulingEvent(result)
	return result.Workload
}

func (s *Service) recordEvent(eventType, actor, workloadID, nodeID, message string, metadata map[string]string) {
	now := s.now().UTC()
	_, _ = s.store.CreateEvent(domain.Event{
		ID:         s.nextID("event", now),
		Timestamp:  now,
		Type:       eventType,
		Actor:      actor,
		WorkloadID: workloadID,
		NodeID:     nodeID,
		Message:    message,
		Metadata:   metadata,
	})
}

func (s *Service) recordScheduledResults(results []store.SchedulingResult) {
	for _, result := range results {
		s.recordSchedulingEvent(result)
	}
}

func (s *Service) recordSchedulingEvent(result store.SchedulingResult) {
	decision := result.Decision
	if decision.Outcome == "" {
		return
	}
	if decision.Outcome == scheduler.OutcomePlaced && decision.SelectedNode != nil {
		s.recordEvent("workload_scheduled", "scheduler", result.Workload.ID, decision.SelectedNode.ID, decision.Reason, nil)
		return
	}
	s.recordEvent("workload_queued", "scheduler", result.Workload.ID, "", decision.Reason, rejectedMetadata(decision.RejectedNodes))
}

func (s *Service) recordAffectedWorkloads(eventType string, workloads []domain.Workload, nodeID string) {
	for _, workload := range workloads {
		s.recordEvent(eventType, "system", workload.ID, nodeID, workload.StatusReason, nil)
	}
}

func rejectedMetadata(rejected []scheduler.RejectedNode) map[string]string {
	if len(rejected) == 0 {
		return nil
	}
	metadata := make(map[string]string, len(rejected))
	for _, node := range rejected {
		metadata[node.NodeID] = node.Reason
	}
	return metadata
}

func (s *Service) nextID(prefix string, now time.Time) string {
	seq := s.seq.Add(1)
	return prefix + "-" + strconv.FormatInt(now.UnixNano(), 36) + "-" + strconv.FormatUint(seq, 36)
}
