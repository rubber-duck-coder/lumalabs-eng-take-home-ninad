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

func TestCORSPreflight(t *testing.T) {
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173"} {
		req := httptest.NewRequest(http.MethodOptions, "/workloads", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()

		NewRouter().ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatalf("expected cors header %q, got %q", origin, rec.Header().Get("Access-Control-Allow-Origin"))
		}
	}
}

func TestCORSRejectsUnknownOriginPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/workloads", nil)
	req.Header.Set("Origin", "https://malicious.example")
	rec := httptest.NewRecorder()

	NewRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
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

func TestSeedDemoDataEndpointPopulatesStore(t *testing.T) {
	memoryStore := store.NewMemoryStore()

	req := httptest.NewRequest(http.MethodPost, "/admin/demo/seed", nil)
	rec := httptest.NewRecorder()
	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["action"] != "seed" {
		t.Fatalf("expected seed action, got %+v", payload)
	}
	if got := int(payload["nodes"].(float64)); got != 6 {
		t.Fatalf("expected 6 nodes, got %d", got)
	}
	if got := int(payload["workloads"].(float64)); got != 3 {
		t.Fatalf("expected 3 workloads, got %d", got)
	}
	if got := int(payload["events"].(float64)); got != 2 {
		t.Fatalf("expected 2 events, got %d", got)
	}
	if len(memoryStore.ListNodes()) != 6 || len(memoryStore.ListWorkloads()) != 3 || len(memoryStore.ListEvents()) != 2 {
		t.Fatalf("expected seeded store state, got nodes=%d workloads=%d events=%d", len(memoryStore.ListNodes()), len(memoryStore.ListWorkloads()), len(memoryStore.ListEvents()))
	}
}

func TestClearDemoDataEndpointEmptiesStore(t *testing.T) {
	memoryStore := store.NewSeededMemoryStore()

	req := httptest.NewRequest(http.MethodPost, "/admin/demo/clear", nil)
	rec := httptest.NewRecorder()
	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["action"] != "clear" {
		t.Fatalf("expected clear action, got %+v", payload)
	}
	if got := int(payload["nodes"].(float64)); got != 6 {
		t.Fatalf("expected 6 nodes cleared, got %d", got)
	}
	if got := int(payload["workloads"].(float64)); got != 3 {
		t.Fatalf("expected 3 workloads cleared, got %d", got)
	}
	if got := int(payload["events"].(float64)); got != 2 {
		t.Fatalf("expected 2 events cleared, got %d", got)
	}
	if len(memoryStore.ListNodes()) != 0 || len(memoryStore.ListWorkloads()) != 0 || len(memoryStore.ListEvents()) != 0 {
		t.Fatalf("expected cleared store state, got nodes=%d workloads=%d events=%d", len(memoryStore.ListNodes()), len(memoryStore.ListWorkloads()), len(memoryStore.ListEvents()))
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

func TestCreateInferenceWorkloadWithReplicas(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	now := fixedTestTime()
	for _, id := range []string{"node-a", "node-b"} {
		_, err := memoryStore.CreateNode(domain.Node{
			ID:            id,
			GPUType:       "A100",
			TotalGPUs:     4,
			AllocatedGPUs: 0,
			Health:        domain.NodeHealthHealthy,
			CapacityClass: domain.CapacityClassOnDemand,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if err != nil {
			t.Fatalf("create node %s: %v", id, err)
		}
	}

	body := bytes.NewBufferString(`{
		"type":"inference",
		"gpu_type":"A100",
		"gpu_count":1,
		"priority":"normal",
		"duration_seconds":300,
		"spot_tolerant":false,
		"replicas":2
	}`)
	req := httptest.NewRequest(http.MethodPost, "/workloads", body)
	rec := httptest.NewRecorder()

	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var workload domain.Workload
	if err := json.NewDecoder(rec.Body).Decode(&workload); err != nil {
		t.Fatalf("decode workload: %v", err)
	}
	if workload.State != domain.WorkloadStateRunning {
		t.Fatalf("expected running inference workload, got %s", workload.State)
	}
	if workload.Replicas != 2 || len(workload.ReplicaPlacements) != 2 {
		t.Fatalf("expected two replica placements, got %+v", workload)
	}
	if workload.ReplicaPlacements[0].NodeID == workload.ReplicaPlacements[1].NodeID {
		t.Fatalf("expected placements on distinct nodes, got %+v", workload.ReplicaPlacements)
	}
}

func TestCreateInferenceWorkloadRejectsNegativeReplicas(t *testing.T) {
	body := bytes.NewBufferString(`{
		"type":"inference",
		"gpu_type":"A100",
		"gpu_count":1,
		"priority":"normal",
		"duration_seconds":300,
		"spot_tolerant":false,
		"replicas":-1
	}`)
	req := httptest.NewRequest(http.MethodPost, "/workloads", body)
	rec := httptest.NewRecorder()

	NewRouterWithStore(store.NewSeededMemoryStore()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_replicas") {
		t.Fatalf("expected invalid_replicas error, got %s", rec.Body.String())
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

func TestSchedulerTickSchedulesPendingWorkloads(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	now := fixedTestTime()
	_, _ = memoryStore.CreateNode(testNode("node-1", "A100", 4, 0, domain.CapacityClassOnDemand, domain.NodeHealthHealthy, nil))
	_, _ = memoryStore.CreateWorkload(testWorkload("w-pending", "A100", 2, domain.WorkloadPriorityHigh, domain.WorkloadStatePending, nil, now))

	req := httptest.NewRequest(http.MethodPost, "/scheduler/tick", nil)
	rec := httptest.NewRecorder()
	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	workload, ok := memoryStore.GetWorkload("w-pending")
	if !ok {
		t.Fatal("expected workload")
	}
	if workload.State != domain.WorkloadStateRunning || workload.Placement == nil || workload.Placement.NodeID != "node-1" {
		t.Fatalf("expected workload running on node-1, got %+v", workload)
	}
}

func TestFailNodeEndpointRequeuesAndReschedulesAffectedWorkload(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	now := fixedTestTime()
	_, _ = memoryStore.CreateNode(testNode("node-a", "A100", 4, 4, domain.CapacityClassOnDemand, domain.NodeHealthHealthy, []string{"w-running"}))
	_, _ = memoryStore.CreateNode(testNode("node-b", "A100", 4, 0, domain.CapacityClassOnDemand, domain.NodeHealthHealthy, nil))
	_, _ = memoryStore.CreateWorkload(testWorkload("w-running", "A100", 4, domain.WorkloadPriorityHigh, domain.WorkloadStateRunning, &domain.Placement{NodeID: "node-a"}, now))

	req := httptest.NewRequest(http.MethodPost, "/admin/nodes/node-a/fail", nil)
	rec := httptest.NewRecorder()
	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	failed, _ := memoryStore.GetNode("node-a")
	if failed.Health != domain.NodeHealthFailed || failed.AllocatedGPUs != 0 {
		t.Fatalf("expected failed node with freed allocation, got %+v", failed)
	}
	workload, _ := memoryStore.GetWorkload("w-running")
	if workload.State != domain.WorkloadStateRunning || workload.Placement == nil || workload.Placement.NodeID != "node-b" {
		t.Fatalf("expected workload rescheduled to node-b, got %+v", workload)
	}
}

func TestFailNodeEndpointReturnsSnakeCaseNodeField(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	now := fixedTestTime()
	_, _ = memoryStore.CreateNode(testNode("node-a", "A100", 4, 4, domain.CapacityClassOnDemand, domain.NodeHealthHealthy, []string{"w-running"}))
	_, _ = memoryStore.CreateWorkload(testWorkload("w-running", "A100", 4, domain.WorkloadPriorityHigh, domain.WorkloadStateRunning, &domain.Placement{NodeID: "node-a"}, now))

	req := httptest.NewRequest(http.MethodPost, "/admin/nodes/node-a/fail", nil)
	rec := httptest.NewRecorder()
	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := payload["node"]; !ok {
		t.Fatalf("expected snake_case node field, got %+v", payload)
	}
	if _, ok := payload["Node"]; ok {
		t.Fatalf("expected no capitalized Node field, got %+v", payload)
	}
}

func TestRecoverNodeEndpointSchedulesPendingWorkload(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	now := fixedTestTime()
	_, _ = memoryStore.CreateNode(testNode("node-1", "A100", 4, 0, domain.CapacityClassOnDemand, domain.NodeHealthRecovering, nil))
	_, _ = memoryStore.CreateWorkload(testWorkload("w-pending", "A100", 2, domain.WorkloadPriorityHigh, domain.WorkloadStatePending, nil, now))

	req := httptest.NewRequest(http.MethodPost, "/admin/nodes/node-1/recover", nil)
	rec := httptest.NewRecorder()
	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	node, _ := memoryStore.GetNode("node-1")
	if node.Health != domain.NodeHealthHealthy {
		t.Fatalf("expected healthy node, got %+v", node)
	}
	workload, _ := memoryStore.GetWorkload("w-pending")
	if workload.State != domain.WorkloadStateRunning {
		t.Fatalf("expected pending workload scheduled after recovery, got %+v", workload)
	}
}

func TestPreemptSpotNodeEndpointRequeuesAffectedWorkload(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	now := fixedTestTime()
	_, _ = memoryStore.CreateNode(testNode("spot-1", "L4", 4, 2, domain.CapacityClassSpot, domain.NodeHealthHealthy, []string{"w-spot"}))
	_, _ = memoryStore.CreateWorkload(testWorkload("w-spot", "L4", 2, domain.WorkloadPriorityNormal, domain.WorkloadStateRunning, &domain.Placement{NodeID: "spot-1"}, now))

	req := httptest.NewRequest(http.MethodPost, "/admin/nodes/spot-1/preempt-spot", nil)
	rec := httptest.NewRecorder()
	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	node, _ := memoryStore.GetNode("spot-1")
	if node.Health != domain.NodeHealthFailed || node.AllocatedGPUs != 0 {
		t.Fatalf("expected preempted spot node failed with freed allocation, got %+v", node)
	}
	workload, _ := memoryStore.GetWorkload("w-spot")
	if workload.State != domain.WorkloadStatePending || workload.Placement != nil {
		t.Fatalf("expected spot workload requeued without placement, got %+v", workload)
	}
}

func TestDisruptionEndpointsReturnErrorsForInvalidRequests(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	_, _ = memoryStore.CreateNode(testNode("node-1", "A100", 4, 0, domain.CapacityClassOnDemand, domain.NodeHealthHealthy, nil))
	router := NewRouterWithStore(memoryStore)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "missing node", path: "/admin/nodes/missing/fail", wantStatus: http.StatusNotFound},
		{name: "non spot preemption", path: "/admin/nodes/node-1/preempt-spot", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d body=%s", tc.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDisruptionEndpointRecordsEvents(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	now := fixedTestTime()
	_, _ = memoryStore.CreateNode(testNode("node-a", "A100", 4, 4, domain.CapacityClassOnDemand, domain.NodeHealthHealthy, []string{"w-running"}))
	_, _ = memoryStore.CreateWorkload(testWorkload("w-running", "A100", 4, domain.WorkloadPriorityHigh, domain.WorkloadStateRunning, &domain.Placement{NodeID: "node-a"}, now))

	req := httptest.NewRequest(http.MethodPost, "/admin/nodes/node-a/fail", nil)
	rec := httptest.NewRecorder()
	NewRouterWithStore(memoryStore).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	events := memoryStore.ListEvents()
	seenNodeFailed := false
	seenDisrupted := false
	for _, event := range events {
		switch event.Type {
		case "node_failed":
			seenNodeFailed = true
		case "workload_disrupted":
			seenDisrupted = true
		}
	}
	if !seenNodeFailed || !seenDisrupted {
		t.Fatalf("expected node_failed and workload_disrupted events, got %+v", events)
	}
}

func fixedTestTime() time.Time {
	return time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
}

func testNode(id, gpuType string, totalGPUs, allocatedGPUs int, capacity domain.CapacityClass, health domain.NodeHealth, running []string) domain.Node {
	now := fixedTestTime()
	return domain.Node{
		ID:                 id,
		GPUType:            gpuType,
		TotalGPUs:          totalGPUs,
		AllocatedGPUs:      allocatedGPUs,
		Health:             health,
		CapacityClass:      capacity,
		RunningWorkloadIDs: running,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func testWorkload(id, gpuType string, gpuCount int, priority domain.WorkloadPriority, state domain.WorkloadState, placement *domain.Placement, now time.Time) domain.Workload {
	return domain.Workload{
		ID:              id,
		Type:            domain.WorkloadTypeTraining,
		GPUType:         gpuType,
		GPUCount:        gpuCount,
		Priority:        priority,
		DurationSeconds: 300,
		SpotTolerant:    true,
		State:           state,
		Placement:       placement,
		SubmittedAt:     now,
		UpdatedAt:       now,
	}
}
