package controlplane

import (
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/fleet"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/workloads"
)

type Service struct {
	store     store.Store
	workloads *workloads.Manager
	fleet     *fleet.Manager
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

func New(appStore store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: appStore, workloads: workloads.New(appStore, now), fleet: fleet.NewManager(appStore, now)}
}

func (s *Service) SubmitWorkload(req SubmitWorkloadRequest) (domain.Workload, error) {
	return s.workloads.Submit(workloads.SubmitRequest{
		Type:            req.Type,
		GPUType:         req.GPUType,
		GPUCount:        req.GPUCount,
		Priority:        req.Priority,
		DurationSeconds: req.DurationSeconds,
		SpotTolerant:    req.SpotTolerant,
		Resumable:       req.Resumable,
		Replicas:        req.Replicas,
	})
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
	return s.workloads.SchedulerTick()
}

func (s *Service) SeedDemoData() (store.DemoDataSummary, error) {
	return s.store.SeedDemoData()
}

func (s *Service) ClearDemoData() (store.DemoDataSummary, error) {
	return s.store.Clear()
}

func (s *Service) FailNode(id string) (store.DisruptionResult, error) {
	return s.fleet.FailNode(id)
}

func (s *Service) RecoverNode(id string) (store.DisruptionResult, error) {
	return s.fleet.RecoverNode(id)
}

func (s *Service) PreemptSpotNode(id string) (store.DisruptionResult, error) {
	return s.fleet.PreemptSpotNode(id)
}
