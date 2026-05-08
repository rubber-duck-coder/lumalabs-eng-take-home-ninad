package fleet

import (
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/events"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

type Manager struct {
	store store.Store
	now   func() time.Time
	evt   *events.Recorder
}

func NewManager(appStore store.Store, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{store: appStore, now: now, evt: events.New(appStore, now)}
}

func (m *Manager) ListNodes() []domain.Node {
	return m.store.ListNodes()
}

func (m *Manager) Summary() Summary {
	return Build(m.store.ListNodes(), m.store.ListWorkloads())
}

func (m *Manager) FailNode(id string) (store.DisruptionResult, error) {
	result, err := m.store.FailNode(id, m.now().UTC())
	if err != nil {
		return store.DisruptionResult{}, err
	}
	m.evt.Record("node_failed", "admin", "", result.Node.ID, "node marked failed", nil)
	m.evt.RecordAffectedWorkloads("workload_disrupted", result.AffectedWorkloads, result.Node.ID)
	m.evt.RecordScheduledResults(result.Scheduled)
	return result, nil
}

func (m *Manager) RecoverNode(id string) (store.DisruptionResult, error) {
	result, err := m.store.RecoverNode(id, m.now().UTC())
	if err != nil {
		return store.DisruptionResult{}, err
	}
	m.evt.Record("node_recovered", "admin", "", result.Node.ID, "node recovered", nil)
	m.evt.RecordScheduledResults(result.Scheduled)
	return result, nil
}

func (m *Manager) PreemptSpotNode(id string) (store.DisruptionResult, error) {
	result, err := m.store.PreemptSpotNode(id, m.now().UTC())
	if err != nil {
		return store.DisruptionResult{}, err
	}
	m.evt.Record("node_spot_preempted", "admin", "", result.Node.ID, "spot node preempted", nil)
	m.evt.RecordAffectedWorkloads("workload_preempted", result.AffectedWorkloads, result.Node.ID)
	m.evt.RecordScheduledResults(result.Scheduled)
	return result, nil
}
