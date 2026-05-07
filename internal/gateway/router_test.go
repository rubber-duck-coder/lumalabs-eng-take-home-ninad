package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	NewRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected healthy response, got %s", rec.Body.String())
	}
}

func TestListSeededNodes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/nodes", nil)
	rec := httptest.NewRecorder()

	NewRouterWithStore(store.NewSeededMemoryStore()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	var nodes []domain.Node
	if err := json.NewDecoder(rec.Body).Decode(&nodes); err != nil {
		t.Fatalf("decode nodes: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected seeded nodes")
	}
}

func TestCreateWorkloadSchedulesWhenCapacityExists(t *testing.T) {
	body := bytes.NewBufferString(`{
		"type":"batch",
		"gpu_type":"A100",
		"gpu_count":2,
		"priority":"normal",
		"duration_seconds":300,
		"spot_tolerant":true
	}`)
	req := httptest.NewRequest(http.MethodPost, "/workloads", body)
	rec := httptest.NewRecorder()

	NewRouterWithStore(store.NewSeededMemoryStore()).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var workload domain.Workload
	if err := json.NewDecoder(rec.Body).Decode(&workload); err != nil {
		t.Fatalf("decode workload: %v", err)
	}
	if workload.State != domain.WorkloadStateRunning {
		t.Fatalf("expected running workload, got %s", workload.State)
	}
	if workload.Placement == nil {
		t.Fatal("expected placement")
	}
	if workload.SchedulingExplanation == "" {
		t.Fatal("expected scheduling explanation")
	}
}

func TestCreateWorkloadQueuesWhenCapacityUnavailable(t *testing.T) {
	body := bytes.NewBufferString(`{
		"type":"training",
		"gpu_type":"L4",
		"gpu_count":4,
		"priority":"high",
		"duration_seconds":300,
		"spot_tolerant":false
	}`)
	req := httptest.NewRequest(http.MethodPost, "/workloads", body)
	rec := httptest.NewRecorder()

	NewRouterWithStore(store.NewSeededMemoryStore()).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var workload domain.Workload
	if err := json.NewDecoder(rec.Body).Decode(&workload); err != nil {
		t.Fatalf("decode workload: %v", err)
	}
	if workload.State != domain.WorkloadStatePending {
		t.Fatalf("expected pending workload, got %s", workload.State)
	}
	if workload.StatusReason == "" {
		t.Fatal("expected queue reason")
	}
}

func TestConcurrentWorkloadSubmissionsDoNotOverAllocate(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	now := fixedTestTime()
	_, err := memoryStore.CreateNode(domain.Node{
		ID:            "node-1",
		GPUType:       "A100",
		TotalGPUs:     4,
		AllocatedGPUs: 0,
		Health:        domain.NodeHealthHealthy,
		CapacityClass: domain.CapacityClassOnDemand,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	router := NewRouterWithStore(memoryStore)
	body := `{
		"type":"inference",
		"gpu_type":"A100",
		"gpu_count":3,
		"priority":"normal",
		"duration_seconds":300,
		"spot_tolerant":false
	}`

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/workloads", strings.NewReader(body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Errorf("expected status %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
			}
		}()
	}
	wg.Wait()

	node, ok := memoryStore.GetNode("node-1")
	if !ok {
		t.Fatal("expected node")
	}
	if node.AllocatedGPUs > node.TotalGPUs {
		t.Fatalf("node over-allocated: %+v", node)
	}

	running := 0
	pending := 0
	for _, workload := range memoryStore.ListWorkloads() {
		switch workload.State {
		case domain.WorkloadStateRunning:
			running++
		case domain.WorkloadStatePending:
			pending++
		}
	}
	if running != 1 || pending != 1 {
		t.Fatalf("expected one running and one pending workload, got running=%d pending=%d", running, pending)
	}
}

func fixedTestTime() time.Time {
	return time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
}
