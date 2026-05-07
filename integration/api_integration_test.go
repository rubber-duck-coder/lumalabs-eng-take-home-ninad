//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/gateway"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

func TestSubmitWorkloadAndInspectFleetEvents(t *testing.T) {
	memory := store.NewMemoryStore()
	now := fixedNow()
	mustCreateNode(t, memory, domain.Node{
		ID:            "node-a100-1",
		GPUType:       "A100",
		TotalGPUs:     4,
		AllocatedGPUs: 0,
		CapacityClass: domain.CapacityClassOnDemand,
		Health:        domain.NodeHealthHealthy,
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	server := httptest.NewServer(gateway.NewRouterWithStore(memory))
	defer server.Close()

	submitBody := map[string]any{
		"type":             "batch",
		"gpu_type":         "A100",
		"gpu_count":        2,
		"priority":         "normal",
		"duration_seconds": 120,
		"spot_tolerant":    true,
	}
	var created domain.Workload
	mustDoJSON(t, http.MethodPost, server.URL+"/workloads", submitBody, http.StatusCreated, &created)

	if created.ID == "" {
		t.Fatal("expected created workload id")
	}
	if created.State != domain.WorkloadStateRunning {
		t.Fatalf("expected running workload, got %s", created.State)
	}
	if created.Placement == nil || created.Placement.NodeID != "node-a100-1" {
		t.Fatalf("expected placement on node-a100-1, got %+v", created.Placement)
	}

	var fetched domain.Workload
	mustDoJSON(t, http.MethodGet, server.URL+"/workloads/"+created.ID, nil, http.StatusOK, &fetched)
	if fetched.State != domain.WorkloadStateRunning || fetched.Placement == nil {
		t.Fatalf("expected fetched workload to be running with placement, got %+v", fetched)
	}

	var summary map[string]any
	mustDoJSON(t, http.MethodGet, server.URL+"/fleet/summary", nil, http.StatusOK, &summary)
	if intFromAny(t, summary["allocated_gpus"]) != 2 {
		t.Fatalf("expected allocated_gpus=2, got %v", summary["allocated_gpus"])
	}
	byState, ok := summary["workloads_by_state"].(map[string]any)
	if !ok || intFromAny(t, byState["running"]) != 1 {
		t.Fatalf("expected workloads_by_state.running=1, got %v", summary["workloads_by_state"])
	}

	var events []domain.Event
	mustDoJSON(t, http.MethodGet, server.URL+"/events", nil, http.StatusOK, &events)
	if countEvents(events, "workload_submitted", created.ID, "") != 1 {
		t.Fatalf("expected one workload_submitted event for %s, got %+v", created.ID, events)
	}
	if countEvents(events, "workload_scheduled", created.ID, "node-a100-1") != 1 {
		t.Fatalf("expected one workload_scheduled event for %s on node-a100-1, got %+v", created.ID, events)
	}
}

func TestDisruptionLifecycleFailPreemptRecover(t *testing.T) {
	memory := store.NewMemoryStore()
	now := fixedNow()

	mustCreateNode(t, memory, domain.Node{
		ID:                 "node-a",
		GPUType:            "A100",
		TotalGPUs:          4,
		AllocatedGPUs:      4,
		CapacityClass:      domain.CapacityClassOnDemand,
		Health:             domain.NodeHealthHealthy,
		RunningWorkloadIDs: []string{"w-main"},
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	mustCreateNode(t, memory, domain.Node{
		ID:            "node-b",
		GPUType:       "A100",
		TotalGPUs:     4,
		AllocatedGPUs: 0,
		CapacityClass: domain.CapacityClassOnDemand,
		Health:        domain.NodeHealthHealthy,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	mustCreateNode(t, memory, domain.Node{
		ID:                 "spot-1",
		GPUType:            "A100",
		TotalGPUs:          4,
		AllocatedGPUs:      2,
		CapacityClass:      domain.CapacityClassSpot,
		Health:             domain.NodeHealthHealthy,
		RunningWorkloadIDs: []string{"w-spot"},
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	mustCreateWorkload(t, memory, domain.Workload{
		ID:              "w-main",
		Type:            domain.WorkloadTypeTraining,
		GPUType:         "A100",
		GPUCount:        4,
		Priority:        domain.WorkloadPriorityHigh,
		DurationSeconds: 300,
		SpotTolerant:    false,
		State:           domain.WorkloadStateRunning,
		Placement:       &domain.Placement{NodeID: "node-a"},
		SubmittedAt:     now,
		UpdatedAt:       now,
	})
	mustCreateWorkload(t, memory, domain.Workload{
		ID:              "w-spot",
		Type:            domain.WorkloadTypeInference,
		GPUType:         "A100",
		GPUCount:        2,
		Priority:        domain.WorkloadPriorityNormal,
		DurationSeconds: 300,
		SpotTolerant:    true,
		State:           domain.WorkloadStateRunning,
		Placement:       &domain.Placement{NodeID: "spot-1"},
		SubmittedAt:     now,
		UpdatedAt:       now,
	})

	server := httptest.NewServer(gateway.NewRouterWithStore(memory))
	defer server.Close()

	mustDoJSON(t, http.MethodPost, server.URL+"/admin/nodes/node-a/fail", nil, http.StatusOK, &map[string]any{})
	var wMain domain.Workload
	mustDoJSON(t, http.MethodGet, server.URL+"/workloads/w-main", nil, http.StatusOK, &wMain)
	if wMain.State != domain.WorkloadStateRunning || wMain.Placement == nil || wMain.Placement.NodeID != "node-b" {
		t.Fatalf("expected w-main moved to node-b after fail, got %+v", wMain)
	}
	nodes := mustListNodes(t, server.URL)
	assertNodeState(t, nodes, "node-a", domain.NodeHealthFailed, 0)
	assertNodeState(t, nodes, "node-b", domain.NodeHealthHealthy, 4)

	mustDoJSON(t, http.MethodPost, server.URL+"/admin/nodes/spot-1/preempt-spot", nil, http.StatusOK, &map[string]any{})
	var wSpot domain.Workload
	mustDoJSON(t, http.MethodGet, server.URL+"/workloads/w-spot", nil, http.StatusOK, &wSpot)
	if wSpot.State != domain.WorkloadStatePending || wSpot.Placement != nil {
		t.Fatalf("expected w-spot pending after preempt, got %+v", wSpot)
	}
	nodes = mustListNodes(t, server.URL)
	assertNodeState(t, nodes, "spot-1", domain.NodeHealthFailed, 0)

	mustDoJSON(t, http.MethodPost, server.URL+"/admin/nodes/node-a/recover", nil, http.StatusOK, &map[string]any{})
	mustDoJSON(t, http.MethodGet, server.URL+"/workloads/w-spot", nil, http.StatusOK, &wSpot)
	if wSpot.State != domain.WorkloadStateRunning || wSpot.Placement == nil || wSpot.Placement.NodeID != "node-a" {
		t.Fatalf("expected w-spot running on node-a after recover, got %+v", wSpot)
	}
	nodes = mustListNodes(t, server.URL)
	assertNodeState(t, nodes, "node-a", domain.NodeHealthHealthy, 2)

	var events []domain.Event
	mustDoJSON(t, http.MethodGet, server.URL+"/events", nil, http.StatusOK, &events)
	if countEvents(events, "node_failed", "", "node-a") != 1 || countEvents(events, "node_spot_preempted", "", "spot-1") != 1 || countEvents(events, "node_recovered", "", "node-a") != 1 {
		t.Fatalf("expected node lifecycle events, got %+v", events)
	}
	if countEvents(events, "workload_disrupted", "w-main", "node-a") != 1 || countEvents(events, "workload_preempted", "w-spot", "spot-1") != 1 {
		t.Fatalf("expected workload disruption events for w-main and w-spot, got %+v", events)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
}

func mustCreateNode(t *testing.T, memory *store.MemoryStore, node domain.Node) {
	t.Helper()
	if _, err := memory.CreateNode(node); err != nil {
		t.Fatalf("create node %s: %v", node.ID, err)
	}
}

func mustCreateWorkload(t *testing.T, memory *store.MemoryStore, workload domain.Workload) {
	t.Helper()
	if _, err := memory.CreateWorkload(workload); err != nil {
		t.Fatalf("create workload %s: %v", workload.ID, err)
	}
}

func mustDoJSON(t *testing.T, method, url string, body any, wantStatus int, out any) {
	t.Helper()

	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http request %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		t.Fatalf("expected status %d, got %d", wantStatus, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func mustListNodes(t *testing.T, baseURL string) []domain.Node {
	t.Helper()
	var nodes []domain.Node
	mustDoJSON(t, http.MethodGet, baseURL+"/nodes", nil, http.StatusOK, &nodes)
	return nodes
}

func assertNodeState(t *testing.T, nodes []domain.Node, id string, health domain.NodeHealth, allocated int) {
	t.Helper()
	for _, node := range nodes {
		if node.ID != id {
			continue
		}
		if node.Health != health || node.AllocatedGPUs != allocated {
			t.Fatalf("unexpected node state for %s: %+v", id, node)
		}
		return
	}
	t.Fatalf("node not found: %s", id)
}

func countEvents(events []domain.Event, typ, workloadID, nodeID string) int {
	count := 0
	for _, event := range events {
		if event.Type != typ {
			continue
		}
		if workloadID != "" && event.WorkloadID != workloadID {
			continue
		}
		if nodeID != "" && event.NodeID != nodeID {
			continue
		}
		count++
	}
	return count
}

func intFromAny(t *testing.T, v any) int {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		t.Fatalf("expected numeric value, got %T", v)
		return 0
	}
}
