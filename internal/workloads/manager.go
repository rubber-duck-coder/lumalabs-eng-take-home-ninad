package workloads

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/events"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

type SubmitRequest struct {
	Type            domain.WorkloadType
	GPUType         string
	GPUCount        int
	Priority        domain.WorkloadPriority
	DurationSeconds int
	SpotTolerant    bool
}

type Manager struct {
	store store.Store
	now   func() time.Time
	evt   *events.Recorder
	seq   atomic.Uint64
}

func New(appStore store.Store, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{store: appStore, now: now, evt: events.New(appStore, now)}
}

func (m *Manager) Submit(req SubmitRequest) (domain.Workload, error) {
	now := m.now().UTC()
	workload := domain.Workload{
		ID:              m.nextID("workload", now),
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

	created, err := m.store.CreateWorkload(workload)
	if err != nil {
		return domain.Workload{}, err
	}

	m.evt.Record("workload_submitted", "system", created.ID, "", "workload submitted", nil)
	return m.schedule(created.ID), nil
}

func (m *Manager) Get(id string) (domain.Workload, bool) {
	return m.store.GetWorkload(id)
}

func (m *Manager) List() []domain.Workload {
	return m.store.ListWorkloads()
}

func (m *Manager) SchedulerTick() ([]store.SchedulingResult, error) {
	results, err := m.store.SchedulePendingWorkloads(m.now().UTC())
	if err != nil {
		return nil, err
	}
	m.evt.Record("scheduler_tick", "scheduler", "", "", "scheduler tick completed", nil)
	m.evt.RecordScheduledResults(results)
	return results, nil
}

func (m *Manager) schedule(id string) domain.Workload {
	workload, ok := m.store.GetWorkload(id)
	if !ok {
		return domain.Workload{}
	}

	now := m.now().UTC()
	result, err := m.store.ScheduleWorkload(id, now)
	if err != nil {
		return workload
	}

	m.evt.RecordSchedulingEvent(result)
	return result.Workload
}

func (m *Manager) nextID(prefix string, now time.Time) string {
	seq := m.seq.Add(1)
	return prefix + "-" + strconv.FormatInt(now.UnixNano(), 36) + "-" + strconv.FormatUint(seq, 36)
}
