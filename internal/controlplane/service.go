package controlplane

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/events"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/fleet"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

type Service struct {
	store store.Store
	now   func() time.Time
	evt   *events.Recorder
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

type FleetSummary = fleet.Summary

func New(appStore store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: appStore, now: now, evt: events.New(appStore, now)}
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

	s.evt.Record("workload_submitted", "system", created.ID, "", "workload submitted", nil)
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
	return fleet.Build(s.store.ListNodes(), s.store.ListWorkloads())
}

func (s *Service) SchedulerTick() ([]store.SchedulingResult, error) {
	results, err := s.store.SchedulePendingWorkloads(s.now().UTC())
	if err != nil {
		return nil, err
	}
	s.evt.Record("scheduler_tick", "scheduler", "", "", "scheduler tick completed", nil)
	s.evt.RecordScheduledResults(results)
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

	s.evt.RecordSchedulingEvent(result)
	return result.Workload
}

func (s *Service) nextID(prefix string, now time.Time) string {
	seq := s.seq.Add(1)
	return prefix + "-" + strconv.FormatInt(now.UnixNano(), 36) + "-" + strconv.FormatUint(seq, 36)
}
