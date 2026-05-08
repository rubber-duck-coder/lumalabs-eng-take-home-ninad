package reconciler

import "testing"

type fakeRunner struct {
	changed int
	err     error
	calls   int
}

func (f *fakeRunner) Reconcile() (int, error) {
	f.calls++
	return f.changed, f.err
}

func TestRunOnceDelegatesToRunner(t *testing.T) {
	runner := &fakeRunner{changed: 2}
	mgr := New(runner)

	changed, err := mgr.RunOnce()
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if changed != 2 {
		t.Fatalf("expected 2 changed nodes, got %d", changed)
	}
	if runner.calls != 1 {
		t.Fatalf("expected one call, got %d", runner.calls)
	}
}
