package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
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
	mux.HandleFunc("POST /scheduler/tick", app.schedulerTick)
	mux.HandleFunc("POST /admin/nodes/{id}/fail", app.failNode)
	mux.HandleFunc("POST /admin/nodes/{id}/recover", app.recoverNode)
	mux.HandleFunc("POST /admin/nodes/{id}/preempt-spot", app.preemptSpotNode)
	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	allowedOrigin := os.Getenv("CORS_ALLOW_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:5173"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allow := origin == "" || origin == allowedOrigin
		if allow {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			if !allow {
				writeError(w, http.StatusForbidden, "origin_not_allowed")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func (app *App) schedulerTick(w http.ResponseWriter, r *http.Request) {
	results, err := app.store.SchedulePendingWorkloads(app.now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scheduler_tick_failed")
		return
	}
	app.recordEvent("scheduler_tick", "scheduler", "", "", "scheduler tick completed", nil)
	for _, result := range results {
		app.recordSchedulingEvent(result)
	}
	writeJSON(w, http.StatusOK, results)
}

func (app *App) failNode(w http.ResponseWriter, r *http.Request) {
	result, err := app.store.FailNode(r.PathValue("id"), app.now().UTC())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	app.recordEvent("node_failed", "admin", "", result.Node.ID, "node marked failed", nil)
	app.recordAffectedWorkloads("workload_disrupted", result.AffectedWorkloads, result.Node.ID)
	app.recordScheduledResults(result.Scheduled)
	writeJSON(w, http.StatusOK, result)
}

func (app *App) recoverNode(w http.ResponseWriter, r *http.Request) {
	result, err := app.store.RecoverNode(r.PathValue("id"), app.now().UTC())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	app.recordEvent("node_recovered", "admin", "", result.Node.ID, "node recovered", nil)
	app.recordScheduledResults(result.Scheduled)
	writeJSON(w, http.StatusOK, result)
}

func (app *App) preemptSpotNode(w http.ResponseWriter, r *http.Request) {
	result, err := app.store.PreemptSpotNode(r.PathValue("id"), app.now().UTC())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	app.recordEvent("node_spot_preempted", "admin", "", result.Node.ID, "spot node preempted", nil)
	app.recordAffectedWorkloads("workload_preempted", result.AffectedWorkloads, result.Node.ID)
	app.recordScheduledResults(result.Scheduled)
	writeJSON(w, http.StatusOK, result)
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

	app.recordSchedulingEvent(result)

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

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, store.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request")
	default:
		writeError(w, http.StatusInternalServerError, "store_error")
	}
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

func (app *App) recordScheduledResults(results []store.SchedulingResult) {
	for _, result := range results {
		app.recordSchedulingEvent(result)
	}
}

func (app *App) recordSchedulingEvent(result store.SchedulingResult) {
	decision := result.Decision
	if decision.Outcome == "" {
		return
	}
	if decision.Outcome == scheduler.OutcomePlaced && decision.SelectedNode != nil {
		app.recordEvent("workload_scheduled", "scheduler", result.Workload.ID, decision.SelectedNode.ID, decision.Reason, nil)
		return
	}
	app.recordEvent("workload_queued", "scheduler", result.Workload.ID, "", decision.Reason, rejectedMetadata(decision.RejectedNodes))
}

func (app *App) recordAffectedWorkloads(eventType string, workloads []domain.Workload, nodeID string) {
	for _, workload := range workloads {
		app.recordEvent(eventType, "system", workload.ID, nodeID, workload.StatusReason, nil)
	}
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
