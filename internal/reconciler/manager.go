package reconciler

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

func New(appStore store.Store, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{store: appStore, now: now, evt: events.New(appStore, now)}
}

func (m *Manager) RunOnce() (int, error) {
	now := m.now().UTC()
	changed := 0

	for _, node := range m.store.ListNodes() {
		if node.Health != domain.NodeHealthRecovering {
			continue
		}

		updated, err := m.store.UpdateNode(node.ID, func(current *domain.Node) error {
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
			m.evt.Record("node_reconciled", "reconciler", "", updated.ID, "node restored by reconciliation loop", nil)
		}
	}

	if changed == 0 {
		return 0, nil
	}

	results, err := m.store.SchedulePendingWorkloads(now)
	if err != nil {
		return changed, err
	}
	m.evt.RecordScheduledResults(results)
	return changed, nil
}
