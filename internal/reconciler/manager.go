package reconciler

type Runner interface {
	Reconcile() (int, error)
}

type Manager struct {
	runner Runner
}

func New(runner Runner) *Manager {
	return &Manager{runner: runner}
}

func (m *Manager) RunOnce() (int, error) {
	return m.runner.Reconcile()
}
