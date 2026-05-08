package controlplane

import (
	"strconv"
	"sync"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/events"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/fleet"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/telemetry"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/workloads"
)

const maxTelemetrySnapshots = 480

type Service struct {
	store     store.Store
	workloads *workloads.Manager
	fleet     *fleet.Manager
	evt       *events.Recorder
	now       func() time.Time
	mu        sync.RWMutex
	telemetry []telemetry.Snapshot
}

type SubmitWorkloadRequest struct {
	Type            domain.WorkloadType
	GPUType         string
	GPUCount        int
	Priority        domain.WorkloadPriority
	DurationSeconds int
	SpotTolerant    bool
	Resumable       bool
	Replicas        int
}

type FleetSummary = fleet.Summary

type SimulationResult struct {
	Scenario    string                   `json:"scenario"`
	Message     string                   `json:"message"`
	Workloads   []domain.Workload        `json:"workloads,omitempty"`
	Disruptions []store.DisruptionResult `json:"disruptions,omitempty"`
	Scheduled   []store.SchedulingResult `json:"scheduled,omitempty"`
}

func New(appStore store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	svc := &Service{
		store:     appStore,
		workloads: workloads.New(appStore, now),
		fleet:     fleet.NewManager(appStore, now),
		evt:       events.New(appStore, now),
		now:       now,
	}
	svc.recordTelemetry()
	return svc
}

func (s *Service) SubmitWorkload(req SubmitWorkloadRequest) (domain.Workload, error) {
	workload, err := s.workloads.Submit(workloads.SubmitRequest{
		Type:            req.Type,
		GPUType:         req.GPUType,
		GPUCount:        req.GPUCount,
		Priority:        req.Priority,
		DurationSeconds: req.DurationSeconds,
		SpotTolerant:    req.SpotTolerant,
		Resumable:       req.Resumable,
		Replicas:        req.Replicas,
	})
	if err != nil {
		return domain.Workload{}, err
	}
	s.recordTelemetry()
	return workload, nil
}

func (s *Service) GetWorkload(id string) (domain.Workload, bool) {
	return s.workloads.Get(id)
}

func (s *Service) ListWorkloads() []domain.Workload {
	return s.workloads.List()
}

func (s *Service) ListNodes() []domain.Node {
	return s.fleet.ListNodes()
}

func (s *Service) ListEvents() []domain.Event {
	return s.store.ListEvents()
}

func (s *Service) FleetSummary() FleetSummary {
	return s.fleet.Summary()
}

func (s *Service) SchedulerTick() ([]store.SchedulingResult, error) {
	results, err := s.workloads.SchedulerTick()
	if err != nil {
		return nil, err
	}
	s.recordTelemetry()
	return results, nil
}

func (s *Service) SeedDemoData() (store.DemoDataSummary, error) {
	summary, err := s.store.SeedDemoData()
	if err != nil {
		return store.DemoDataSummary{}, err
	}
	s.recordTelemetry()
	return summary, nil
}

func (s *Service) ClearDemoData() (store.DemoDataSummary, error) {
	summary, err := s.store.Clear()
	if err != nil {
		return store.DemoDataSummary{}, err
	}
	s.recordTelemetry()
	return summary, nil
}

func (s *Service) FailNode(id string) (store.DisruptionResult, error) {
	result, err := s.fleet.FailNode(id)
	if err != nil {
		return store.DisruptionResult{}, err
	}
	s.recordTelemetry()
	return result, nil
}

func (s *Service) RecoverNode(id string) (store.DisruptionResult, error) {
	result, err := s.fleet.RecoverNode(id)
	if err != nil {
		return store.DisruptionResult{}, err
	}
	s.recordTelemetry()
	return result, nil
}

func (s *Service) PreemptSpotNode(id string) (store.DisruptionResult, error) {
	result, err := s.fleet.PreemptSpotNode(id)
	if err != nil {
		return store.DisruptionResult{}, err
	}
	s.recordTelemetry()
	return result, nil
}

func (s *Service) RunSimulation(scenario string) (SimulationResult, error) {
	if !isKnownSimulation(scenario) {
		return SimulationResult{}, store.ErrInvalid
	}

	s.evt.Record("simulation_started", "admin", "", "", simulationStartedMessage(scenario), map[string]string{"scenario": scenario})

	var result SimulationResult
	var err error

	switch scenario {
	case "sudden-inference-spike":
		result, err = s.runSuddenInferenceSpike()
	case "spot-preemption":
		result, err = s.runSpotPreemptionSimulation()
	case "node-failures":
		result, err = s.runNodeFailuresSimulation()
	case "capacity-exhausted":
		result, err = s.runCapacityExhaustionSimulation()
	}
	if err != nil {
		s.evt.Record("simulation_failed", "admin", "", "", simulationFailedMessage(scenario, err), map[string]string{
			"scenario": scenario,
			"error":    err.Error(),
		})
		return SimulationResult{}, err
	}

	result.Scenario = scenario
	s.recordSimulationNarrative(scenario, result)
	s.evt.Record("simulation_completed", "admin", "", "", result.Message, map[string]string{"scenario": scenario})
	s.recordTelemetry()
	return result, nil
}

func (s *Service) Reconcile() (int, error) {
	now := s.now().UTC()
	changed := 0

	completed, err := s.store.CompleteExpiredWorkloads(now)
	if err != nil {
		return changed, err
	}
	for _, workload := range completed {
		changed++
		s.evt.Record("workload_completed", "reconciler", workload.ID, "", "workload completed after requested duration; freed GPU capacity", map[string]string{
			"duration_seconds": stringInt(workload.DurationSeconds),
		})
	}

	for _, node := range s.store.ListNodes() {
		if node.Health != domain.NodeHealthRecovering {
			continue
		}

		updated, err := s.store.UpdateNode(node.ID, func(current *domain.Node) error {
			if current.Health != domain.NodeHealthRecovering {
				return nil
			}
			current.Health = domain.NodeHealthHealthy
			current.UpdatedAt = now
			return nil
		})
		if err != nil {
			return changed, err
		}
		if updated.Health == domain.NodeHealthHealthy {
			changed++
			s.evt.Record("node_reconciled", "reconciler", "", updated.ID, "node restored by reconciliation loop", nil)
		}
	}

	if changed > 0 {
		results, err := s.store.SchedulePendingWorkloads(now)
		if err != nil {
			return changed, err
		}
		s.evt.RecordScheduledResults(results)
	}

	s.recordTelemetry()
	return changed, nil
}

func (s *Service) RecordTelemetry() {
	s.recordTelemetry()
}

func isKnownSimulation(scenario string) bool {
	switch scenario {
	case "sudden-inference-spike", "spot-preemption", "node-failures", "capacity-exhausted":
		return true
	default:
		return false
	}
}

func simulationStartedMessage(scenario string) string {
	return titleizeScenario(scenario) + " simulation started"
}

func simulationFailedMessage(scenario string, err error) string {
	return titleizeScenario(scenario) + " simulation failed: " + err.Error()
}

func titleizeScenario(scenario string) string {
	switch scenario {
	case "sudden-inference-spike":
		return "Sudden inference spike"
	case "spot-preemption":
		return "Spot preemption"
	case "node-failures":
		return "Node failures"
	case "capacity-exhausted":
		return "Capacity exhaustion"
	default:
		return scenario
	}
}

func (s *Service) recordSimulationNarrative(scenario string, result SimulationResult) {
	for _, workload := range result.Workloads {
		metadata := map[string]string{
			"scenario":  scenario,
			"state":     string(workload.State),
			"gpu_type":  workload.GPUType,
			"gpu_count": stringInt(workload.GPUCount),
		}
		nodeID := ""
		if workload.Placement != nil {
			nodeID = workload.Placement.NodeID
			metadata["node"] = nodeID
		}
		if workload.StatusReason != "" {
			metadata["why"] = workload.StatusReason
		}
		s.evt.Record("simulation_workload", "admin", workload.ID, nodeID, simulationWorkloadMessage(scenario, workload), metadata)
	}

	for _, disruption := range result.Disruptions {
		metadata := map[string]string{
			"scenario":           scenario,
			"node":               disruption.Node.ID,
			"node_health":        string(disruption.Node.Health),
			"affected_workloads": stringInt(len(disruption.AffectedWorkloads)),
			"scheduled":          stringInt(len(disruption.Scheduled)),
		}
		s.evt.Record("simulation_disruption", "admin", "", disruption.Node.ID, simulationDisruptionMessage(scenario, disruption), metadata)
	}
}

func simulationWorkloadMessage(scenario string, workload domain.Workload) string {
	prefix := titleizeScenario(scenario)
	if workload.Placement != nil {
		return prefix + " placed " + workload.ID + " on " + workload.Placement.NodeID + " because " + nonEmpty(workload.StatusReason, workload.SchedulingExplanation, "the scheduler found eligible capacity")
	}
	return prefix + " queued " + workload.ID + " because " + nonEmpty(workload.StatusReason, workload.SchedulingExplanation, "no eligible capacity was available")
}

func simulationDisruptionMessage(scenario string, disruption store.DisruptionResult) string {
	return titleizeScenario(scenario) + " changed " + disruption.Node.ID + " to " + string(disruption.Node.Health) + "; affected " + stringInt(len(disruption.AffectedWorkloads)) + " workload(s), rescheduled " + stringInt(len(disruption.Scheduled))
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringInt(value int) string {
	return strconv.Itoa(value)
}

func (s *Service) runSuddenInferenceSpike() (SimulationResult, error) {
	workloads := make([]domain.Workload, 0, 3)
	for i := 0; i < 3; i++ {
		workload, err := s.SubmitWorkload(SubmitWorkloadRequest{
			Type:            domain.WorkloadTypeInference,
			GPUType:         "L4",
			GPUCount:        1,
			Priority:        domain.WorkloadPriorityHigh,
			DurationSeconds: 8,
			SpotTolerant:    true,
			Resumable:       true,
			Replicas:        1,
		})
		if err != nil {
			return SimulationResult{}, err
		}
		workloads = append(workloads, workload)
	}
	return SimulationResult{
		Message:   "submitted inference spike and consumed schedulable L4 capacity",
		Workloads: workloads,
	}, nil
}

func (s *Service) runSpotPreemptionSimulation() (SimulationResult, error) {
	node, ok := firstHealthyNode(s.ListNodes(), func(node domain.Node) bool {
		return node.CapacityClass == domain.CapacityClassSpot
	})
	if !ok {
		return SimulationResult{}, store.ErrNotFound
	}
	disruption, err := s.PreemptSpotNode(node.ID)
	if err != nil {
		return SimulationResult{}, err
	}
	return SimulationResult{
		Message:     "preempted a healthy spot node",
		Disruptions: []store.DisruptionResult{disruption},
		Scheduled:   disruption.Scheduled,
	}, nil
}

func (s *Service) runNodeFailuresSimulation() (SimulationResult, error) {
	disruptions := make([]store.DisruptionResult, 0, 2)
	for _, node := range s.ListNodes() {
		if node.Health != domain.NodeHealthHealthy || node.AllocatedGPUs == 0 {
			continue
		}
		disruption, err := s.FailNode(node.ID)
		if err != nil {
			return SimulationResult{}, err
		}
		disruptions = append(disruptions, disruption)
		if len(disruptions) == 2 {
			break
		}
	}
	if len(disruptions) == 0 {
		for _, node := range s.ListNodes() {
			if node.Health != domain.NodeHealthHealthy {
				continue
			}
			disruption, err := s.FailNode(node.ID)
			if err != nil {
				return SimulationResult{}, err
			}
			disruptions = append(disruptions, disruption)
			if len(disruptions) == 2 {
				break
			}
		}
	}
	if len(disruptions) == 0 {
		return SimulationResult{}, store.ErrNotFound
	}
	return SimulationResult{
		Message:     "failed healthy nodes",
		Disruptions: disruptions,
	}, nil
}

func (s *Service) runCapacityExhaustionSimulation() (SimulationResult, error) {
	workloads := make([]domain.Workload, 0, 4)
	for i := 0; i < 4; i++ {
		workload, err := s.SubmitWorkload(SubmitWorkloadRequest{
			Type:            domain.WorkloadTypeTraining,
			GPUType:         "A100",
			GPUCount:        8,
			Priority:        domain.WorkloadPriorityHigh,
			DurationSeconds: 12,
			SpotTolerant:    false,
			Resumable:       true,
			Replicas:        1,
		})
		if err != nil {
			return SimulationResult{}, err
		}
		workloads = append(workloads, workload)
	}
	return SimulationResult{
		Message:   "submitted training workloads until A100 capacity became constrained",
		Workloads: workloads,
	}, nil
}

func firstHealthyNode(nodes []domain.Node, predicate func(domain.Node) bool) (domain.Node, bool) {
	for _, node := range nodes {
		if node.Health == domain.NodeHealthHealthy && predicate(node) {
			return node, true
		}
	}
	return domain.Node{}, false
}

func (s *Service) TelemetryHistory(limit int) []telemetry.Snapshot {
	s.mu.RLock()
	telemetryCount := len(s.telemetry)
	if telemetryCount == 0 {
		s.mu.RUnlock()
		return []telemetry.Snapshot{s.captureTelemetrySnapshot()}
	}
	if limit <= 0 || limit >= telemetryCount {
		out := cloneTelemetry(s.telemetry)
		s.mu.RUnlock()
		return out
	}
	start := telemetryCount - limit
	out := cloneTelemetry(s.telemetry[start:])
	s.mu.RUnlock()
	return out
}

func (s *Service) recordTelemetry() {
	snapshot := s.captureTelemetrySnapshot()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.telemetry = append(s.telemetry, snapshot)
	if len(s.telemetry) > maxTelemetrySnapshots {
		s.telemetry = append([]telemetry.Snapshot(nil), s.telemetry[len(s.telemetry)-maxTelemetrySnapshots:]...)
	}
}

func (s *Service) captureTelemetrySnapshot() telemetry.Snapshot {
	return telemetry.Capture(s.now().UTC(), s.ListNodes(), s.ListWorkloads())
}

func cloneTelemetry(snapshots []telemetry.Snapshot) []telemetry.Snapshot {
	if len(snapshots) == 0 {
		return nil
	}
	out := make([]telemetry.Snapshot, len(snapshots))
	copy(out, snapshots)
	return out
}
