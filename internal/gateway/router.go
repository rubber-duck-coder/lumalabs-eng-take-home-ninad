package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/scheduler"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

type App struct {
	store *store.MemoryStore
	now   func() time.Time
	seq   atomic.Uint64
}

func NewRouter() http.Handler {
	return NewRouterWithStore(store.NewSeededMemoryStore())
}

func NewRouterWithStore(memoryStore *store.MemoryStore) http.Handler {
	app := &App{store: memoryStore, now: time.Now}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /nodes", app.listNodes)
	mux.HandleFunc("GET /fleet/summary", app.fleetSummary)
	mux.HandleFunc("GET /events", app.listEvents)
	mux.HandleFunc("GET /workloads", app.listWorkloads)
	mux.HandleFunc("POST /workloads", app.createWorkload)
	mux.HandleFunc("GET /workloads/{id}", app.getWorkload)
	return mux
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createWorkloadRequest struct {
	Type            domain.WorkloadType     `json:"type"`
	GPUType         string                  `json:"gpu_type"`
	GPUCount        int                     `json:"gpu_count"`
	Priority        domain.WorkloadPriority `json:"priority"`
	DurationSeconds int                     `json:"duration_seconds"`
	SpotTolerant    bool                    `json:"spot_tolerant"`
}

func (app *App) createWorkload(w http.ResponseWriter, r *http.Request) {
	var req createWorkloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := validateWorkloadRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := app.now().UTC()
	workload := domain.Workload{
		ID:              app.nextID("workload", now),
		Type:            req.Type,
		GPUType:         strings.ToUpper(req.GPUType),
		GPUCount:        req.GPUCount,
		Priority:        req.Priority,
		DurationSeconds: req.DurationSeconds,
		SpotTolerant:    req.SpotTolerant,
		State:           domain.WorkloadStatePending,
		SubmittedAt:     now,
		UpdatedAt:       now,
	}

	created, err := app.store.CreateWorkload(workload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create_workload_failed")
		return
	}

	app.recordEvent("workload_submitted", "system", created.ID, "", "workload submitted", nil)
	writeJSON(w, http.StatusCreated, app.scheduleWorkload(created.ID))
}

func (app *App) listWorkloads(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.store.ListWorkloads())
}

func (app *App) getWorkload(w http.ResponseWriter, r *http.Request) {
	workload, ok := app.store.GetWorkload(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "workload_not_found")
		return
	}
	writeJSON(w, http.StatusOK, workload)
}

func (app *App) listNodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.store.ListNodes())
}

func (app *App) listEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.store.ListEvents())
}

func (app *App) fleetSummary(w http.ResponseWriter, r *http.Request) {
	nodes := app.store.ListNodes()
	workloads := app.store.ListWorkloads()

	var total, allocated int
	byGPU := map[string]map[string]int{}
	for _, node := range nodes {
		total += node.TotalGPUs
		allocated += node.AllocatedGPUs
		if _, ok := byGPU[node.GPUType]; !ok {
			byGPU[node.GPUType] = map[string]int{"total": 0, "allocated": 0}
		}
		byGPU[node.GPUType]["total"] += node.TotalGPUs
		byGPU[node.GPUType]["allocated"] += node.AllocatedGPUs
	}

	byState := map[domain.WorkloadState]int{}
	for _, workload := range workloads {
		byState[workload.State]++
	}

	utilization := 0.0
	if total > 0 {
		utilization = float64(allocated) / float64(total) * 100
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total_gpus":          total,
		"allocated_gpus":      allocated,
		"available_gpus":      total - allocated,
		"utilization_percent": utilization,
		"gpu_types":           byGPU,
		"workloads_by_state":  byState,
	})
}

func (app *App) scheduleWorkload(id string) domain.Workload {
	workload, ok := app.store.GetWorkload(id)
	if !ok {
		return domain.Workload{}
	}

	now := app.now().UTC()
	result, err := app.store.ScheduleWorkload(id, now)
	if err != nil {
		return workload
	}

	decision := result.Decision
	if decision.Outcome == scheduler.OutcomePlaced && decision.SelectedNode != nil {
		app.recordEvent("workload_scheduled", "scheduler", workload.ID, decision.SelectedNode.ID, decision.Reason, nil)
	} else {
		app.recordEvent("workload_queued", "scheduler", workload.ID, "", decision.Reason, rejectedMetadata(decision.RejectedNodes))
	}

	return result.Workload
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func validateWorkloadRequest(req createWorkloadRequest) error {
	switch req.Type {
	case domain.WorkloadTypeTraining, domain.WorkloadTypeInference, domain.WorkloadTypeBatch:
	default:
		return fmt.Errorf("invalid_type")
	}
	switch strings.ToUpper(req.GPUType) {
	case "H100", "A100", "L4":
	default:
		return fmt.Errorf("invalid_gpu_type")
	}
	if req.GPUCount <= 0 {
		return fmt.Errorf("invalid_gpu_count")
	}
	switch req.Priority {
	case domain.WorkloadPriorityLow, domain.WorkloadPriorityNormal, domain.WorkloadPriorityHigh:
	default:
		return fmt.Errorf("invalid_priority")
	}
	if req.DurationSeconds <= 0 {
		return fmt.Errorf("invalid_duration")
	}
	return nil
}

func (app *App) recordEvent(eventType, actor, workloadID, nodeID, message string, metadata map[string]string) {
	now := app.now().UTC()
	_, _ = app.store.CreateEvent(domain.Event{
		ID:         app.nextID("event", now),
		Timestamp:  now,
		Type:       eventType,
		Actor:      actor,
		WorkloadID: workloadID,
		NodeID:     nodeID,
		Message:    message,
		Metadata:   metadata,
	})
}

func rejectedMetadata(rejected []scheduler.RejectedNode) map[string]string {
	if len(rejected) == 0 {
		return nil
	}
	metadata := make(map[string]string, len(rejected))
	for _, node := range rejected {
		metadata[node.NodeID] = node.Reason
	}
	return metadata
}

func (app *App) nextID(prefix string, now time.Time) string {
	seq := app.seq.Add(1)
	return prefix + "-" + strconv.FormatInt(now.UnixNano(), 36) + "-" + strconv.FormatUint(seq, 36)
}
