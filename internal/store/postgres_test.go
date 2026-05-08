package store

import (
	"context"
	"os"
	"testing"
	"time"
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
