package controlplane

import (
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/events"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/fleet"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/workloads"
)

type Service struct {
	store     store.Store
	now       func() time.Time
	evt       *events.Recorder
	workloads *workloads.Manager
}

type SubmitWorkloadRequest struct {
	Type            domain.WorkloadType
	GPUType         string
	GPUCount        int
	Priority        domain.WorkloadPriority
	DurationSeconds int
	SpotTolerant    bool
}

type FleetSummary = fleet.Summary

func New(appStore store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: appStore, now: now, evt: events.New(appStore, now), workloads: workloads.New(appStore, now)}
}

func (s *Service) SubmitWorkload(req SubmitWorkloadRequest) (domain.Workload, error) {
	return s.workloads.Submit(workloads.SubmitRequest{
		Type:            req.Type,
		GPUType:         req.GPUType,
		GPUCount:        req.GPUCount,
		Priority:        req.Priority,
		DurationSeconds: req.DurationSeconds,
		SpotTolerant:    req.SpotTolerant,
	})
}

func (s *Service) GetWorkload(id string) (domain.Workload, bool) {
	return s.workloads.Get(id)
}

func (s *Service) ListWorkloads() []domain.Workload {
	return s.workloads.List()
}

func (s *Service) ListNodes() []domain.Node {
	return s.store.ListNodes()
}

func (s *Service) ListEvents() []domain.Event {
	return s.store.ListEvents()
}

func (s *Service) FleetSummary() FleetSummary {
	return fleet.Build(s.store.ListNodes(), s.store.ListWorkloads())
}

func (s *Service) SchedulerTick() ([]store.SchedulingResult, error) {
	return s.workloads.SchedulerTick()
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
	s.evt.Record("node_failed", "admin", "", result.Node.ID, "node marked failed", nil)
	s.evt.RecordAffectedWorkloads("workload_disrupted", result.AffectedWorkloads, result.Node.ID)
	s.evt.RecordScheduledResults(result.Scheduled)
	return result, nil
}

func (s *Service) RecoverNode(id string) (store.DisruptionResult, error) {
	result, err := s.store.RecoverNode(id, s.now().UTC())
	if err != nil {
		return store.DisruptionResult{}, err
	}
	s.evt.Record("node_recovered", "admin", "", result.Node.ID, "node recovered", nil)
	s.evt.RecordScheduledResults(result.Scheduled)
	return result, nil
}

func (s *Service) PreemptSpotNode(id string) (store.DisruptionResult, error) {
	result, err := s.store.PreemptSpotNode(id, s.now().UTC())
	if err != nil {
		return store.DisruptionResult{}, err
	}
	s.evt.Record("node_spot_preempted", "admin", "", result.Node.ID, "spot node preempted", nil)
	s.evt.RecordAffectedWorkloads("workload_preempted", result.AffectedWorkloads, result.Node.ID)
	s.evt.RecordScheduledResults(result.Scheduled)
	return result, nil
}
