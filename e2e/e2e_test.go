//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

type workload struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	GPUType      string `json:"gpu_type"`
	GPUCount     int    `json:"gpu_count"`
	Priority     string `json:"priority"`
	Replicas     int    `json:"replicas"`
	State        string `json:"state"`
	StatusReason string `json:"status_reason"`
	Placement    *struct {
		NodeID string `json:"node_id"`
	} `json:"placement"`
	ReplicaPlacements []struct {
		NodeID string `json:"node_id"`
	} `json:"replica_placements"`
}

type node struct {
	ID            string `json:"id"`
	GPUType       string `json:"gpu_type"`
	CapacityClass string `json:"capacity_class"`
	Health        string `json:"health"`
	AllocatedGPUs int    `json:"allocated_gpus"`
	TotalGPUs     int    `json:"total_gpus"`
	Region        string `json:"region"`
	Zone          string `json:"zone"`
	Provider      string `json:"provider"`
}

type event struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	WorkloadID string `json:"workload_id"`
	NodeID     string `json:"node_id"`
	Message    string `json:"message"`
}

func TestLiveCoreFlow(t *testing.T) {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		t.Skip("BASE_URL not set")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	mustDoJSON(t, client, http.MethodPost, baseURL+"/admin/demo/seed", map[string]any{}, nil)
	nodes := mustListNodes(t, client, baseURL)
	if len(nodes) == 0 {
		t.Fatal("expected seeded nodes")
	}

	var created workload
	mustDoJSON(t, client, http.MethodPost, baseURL+"/workloads", map[string]any{
		"type":             "inference",
		"gpu_type":         "A100",
		"gpu_count":        1,
		"priority":         "normal",
		"duration_seconds": 300,
		"spot_tolerant":    true,
		"replicas":         2,
	}, &created)

	if created.State != "running" {
		t.Fatalf("expected running workload, got %+v", created)
	}
	if created.Replicas != 2 || len(created.ReplicaPlacements) != 2 {
		t.Fatalf("expected two replica placements, got %+v", created)
	}
	if created.ReplicaPlacements[0].NodeID == created.ReplicaPlacements[1].NodeID {
		t.Fatalf("expected distinct replica placements, got %+v", created)
	}

	var fetched workload
	mustDoJSON(t, client, http.MethodGet, baseURL+"/workloads/"+created.ID, nil, &fetched)
	if fetched.State != "running" || len(fetched.ReplicaPlacements) != 2 {
		t.Fatalf("expected fetched running workload with replica placements, got %+v", fetched)
	}

	var summary map[string]any
	mustDoJSON(t, client, http.MethodGet, baseURL+"/fleet/summary", nil, &summary)
	if got := intFromAny(summary["workloads_by_state"].(map[string]any)["running"]); got < 1 {
		t.Fatalf("expected at least one running workload, got %+v", summary["workloads_by_state"])
	}

	var events []event
	mustDoJSON(t, client, http.MethodGet, baseURL+"/events", nil, &events)
	if !hasEvent(events, "workload_submitted", created.ID, "") {
		t.Fatalf("expected workload_submitted event for %s", created.ID)
	}
	if !hasEvent(events, "workload_scheduled", created.ID, "") {
		t.Fatalf("expected workload_scheduled event for %s", created.ID)
	}
}

func TestLiveDisruptionFlow(t *testing.T) {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		t.Skip("BASE_URL not set")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	mustDoJSON(t, client, http.MethodPost, baseURL+"/admin/demo/seed", map[string]any{}, nil)

	nodes := mustListNodes(t, client, baseURL)
	var target node
	for _, n := range nodes {
		if n.Health == "healthy" {
			target = n
			break
		}
	}
	if target.ID == "" {
		t.Fatal("expected a healthy node to disrupt")
	}

	var before workload
	mustDoJSON(t, client, http.MethodPost, baseURL+"/workloads", map[string]any{
		"type":             "training",
		"gpu_type":         target.GPUType,
		"gpu_count":        1,
		"priority":         "normal",
		"duration_seconds": 300,
		"spot_tolerant":    false,
	}, &before)

	var disrupted map[string]any
	mustDoJSON(t, client, http.MethodPost, baseURL+"/admin/nodes/"+target.ID+"/fail", nil, &disrupted)

	var after workload
	mustDoJSON(t, client, http.MethodGet, baseURL+"/workloads/"+before.ID, nil, &after)
	if after.State != "running" && after.State != "pending" {
		t.Fatalf("expected workload to remain visible after disruption, got %+v", after)
	}

	var events []event
	mustDoJSON(t, client, http.MethodGet, baseURL+"/events", nil, &events)
	if !hasEvent(events, "node_failed", "", target.ID) {
		t.Fatalf("expected node_failed event for %s", target.ID)
	}
}

func TestLiveRebalanceFlow(t *testing.T) {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		t.Skip("BASE_URL not set")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	mustDoJSON(t, client, http.MethodPost, baseURL+"/admin/demo/seed", map[string]any{}, nil)

	var batch workload
	mustDoJSON(t, client, http.MethodPost, baseURL+"/workloads", map[string]any{
		"type":             "batch",
		"gpu_type":         "A100",
		"gpu_count":        4,
		"priority":         "normal",
		"duration_seconds": 600,
		"spot_tolerant":    false,
	}, &batch)
	if batch.State != "running" {
		t.Fatalf("expected batch workload to run before rebalance, got %+v", batch)
	}

	var inference workload
	mustDoJSON(t, client, http.MethodPost, baseURL+"/workloads", map[string]any{
		"type":             "inference",
		"gpu_type":         "A100",
		"gpu_count":        4,
		"priority":         "normal",
		"duration_seconds": 300,
		"spot_tolerant":    false,
	}, &inference)

	if inference.State != "running" {
		t.Fatalf("expected inference workload to run after rebalance, got %+v", inference)
	}
	if inference.Placement == nil || inference.Placement.NodeID == "" {
		t.Fatalf("expected inference workload placement, got %+v", inference)
	}

	var fetchedBatch workload
	mustDoJSON(t, client, http.MethodGet, baseURL+"/workloads/"+batch.ID, nil, &fetchedBatch)
	if fetchedBatch.State != "pending" {
		t.Fatalf("expected batch workload to be requeued, got %+v", fetchedBatch)
	}
	if fetchedBatch.StatusReason == "" {
		t.Fatalf("expected batch workload to carry a reason after rebalance, got %+v", fetchedBatch)
	}
}

func mustListNodes(t *testing.T, client *http.Client, baseURL string) []node {
	t.Helper()
	var nodes []node
	mustDoJSON(t, client, http.MethodGet, baseURL+"/nodes", nil, &nodes)
	return nodes
}

func mustDoJSON(t *testing.T, client *http.Client, method, url string, body any, out any) {
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

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errPayload map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errPayload)
		t.Fatalf("unexpected status %d for %s %s: %+v", resp.StatusCode, method, url, errPayload)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func hasEvent(events []event, typ, workloadID, nodeID string) bool {
	for _, e := range events {
		if e.Type != typ {
			continue
		}
		if workloadID != "" && e.WorkloadID != workloadID {
			continue
		}
		if nodeID != "" && e.NodeID != nodeID {
			continue
		}
		return true
	}
	return false
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		panic(fmt.Sprintf("expected numeric value, got %T", v))
	}
}
