package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
)

func TestConfiguredStoreFallsBackToMemory(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	s, err := NewConfiguredStore(context.Background())
	if err != nil {
		t.Fatalf("configured store: %v", err)
	}
	if _, ok := s.(*MemoryStore); !ok {
		t.Fatalf("expected memory store fallback, got %T", s)
	}
}

func TestPostgresStoreSmoke(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s, err := NewPostgresStore(ctx, dsn, true)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer s.Close()

	if got := len(s.ListNodes()); got == 0 {
		t.Fatal("expected seeded postgres store to contain nodes")
	}

	summary, err := s.Clear()
	if err != nil {
		t.Fatalf("clear postgres store: %v", err)
	}
	if summary.Nodes == 0 || summary.Workloads == 0 || summary.Events == 0 {
		t.Fatalf("expected clear summary to report seeded data, got %+v", summary)
	}
	if got := len(s.ListNodes()); got != 0 {
		t.Fatalf("expected cleared postgres store to be empty, got %d nodes", got)
	}
}

func TestPostgresStoreRoundTripsInferenceReplicas(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s, err := NewPostgresStore(ctx, dsn, false)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer s.Close()

	if _, err := s.Clear(); err != nil {
		t.Fatalf("clear postgres store: %v", err)
	}

	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"node-a", "node-b"} {
		if _, err := s.CreateNode(domain.Node{
			ID:            id,
			GPUType:       "A100",
			TotalGPUs:     1,
			AllocatedGPUs: 0,
			Health:        domain.NodeHealthHealthy,
			CapacityClass: domain.CapacityClassOnDemand,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			t.Fatalf("create node %s: %v", id, err)
		}
	}

	created, err := s.CreateWorkload(domain.Workload{
		ID:          "svc-1",
		Type:        domain.WorkloadTypeInference,
		GPUType:     "A100",
		GPUCount:    1,
		Priority:    domain.WorkloadPriorityNormal,
		Replicas:    2,
		State:       domain.WorkloadStatePending,
		SubmittedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	if created.Replicas != 2 {
		t.Fatalf("expected replicas to persist on create, got %+v", created)
	}

	result, err := s.ScheduleWorkload("svc-1", now.Add(time.Second))
	if err != nil {
		t.Fatalf("schedule workload: %v", err)
	}
	if result.Workload.State != domain.WorkloadStateRunning {
		t.Fatalf("expected running workload, got %+v", result.Workload)
	}
	if len(result.Workload.ReplicaPlacements) != 2 {
		t.Fatalf("expected two replica placements, got %+v", result.Workload.ReplicaPlacements)
	}

	fetched, ok := s.GetWorkload("svc-1")
	if !ok {
		t.Fatal("expected fetched workload")
	}
	if fetched.Replicas != 2 {
		t.Fatalf("expected replicas to round-trip, got %+v", fetched)
	}
	if len(fetched.ReplicaPlacements) != 2 {
		t.Fatalf("expected replica placements to round-trip, got %+v", fetched.ReplicaPlacements)
	}
}
