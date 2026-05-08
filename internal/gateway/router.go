package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/controlplane"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

type App struct {
	cp *controlplane.Service
}

func NewRouter() http.Handler {
	return NewRouterWithStore(store.NewSeededMemoryStore())
}

func NewRouterWithStore(appStore store.Store) http.Handler {
	app := &App{cp: controlplane.New(appStore, nil)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /nodes", app.listNodes)
	mux.HandleFunc("GET /fleet/summary", app.fleetSummary)
	mux.HandleFunc("GET /events", app.listEvents)
	mux.HandleFunc("GET /workloads", app.listWorkloads)
	mux.HandleFunc("POST /workloads", app.createWorkload)
	mux.HandleFunc("GET /workloads/{id}", app.getWorkload)
	mux.HandleFunc("POST /scheduler/tick", app.schedulerTick)
	mux.HandleFunc("POST /admin/demo/seed", app.seedDemoData)
	mux.HandleFunc("POST /admin/demo/clear", app.clearDemoData)
	mux.HandleFunc("POST /admin/nodes/{id}/fail", app.failNode)
	mux.HandleFunc("POST /admin/nodes/{id}/recover", app.recoverNode)
	mux.HandleFunc("POST /admin/nodes/{id}/preempt-spot", app.preemptSpotNode)
	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	allowedOrigins := allowedOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allow := origin == "" || allowedOrigins[origin]
		if allow {
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
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

func allowedOrigins() map[string]bool {
	raw := os.Getenv("CORS_ALLOW_ORIGIN")
	if raw == "" {
		raw = "http://localhost:5173,http://127.0.0.1:5173"
	}

	allowed := make(map[string]bool)
	for _, origin := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(origin)
		if trimmed != "" {
			allowed[trimmed] = true
		}
	}
	return allowed
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

	created, err := app.cp.SubmitWorkload(controlplane.SubmitWorkloadRequest{
		Type:            req.Type,
		GPUType:         req.GPUType,
		GPUCount:        req.GPUCount,
		Priority:        req.Priority,
		DurationSeconds: req.DurationSeconds,
		SpotTolerant:    req.SpotTolerant,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create_workload_failed")
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (app *App) listWorkloads(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.cp.ListWorkloads())
}

func (app *App) getWorkload(w http.ResponseWriter, r *http.Request) {
	workload, ok := app.cp.GetWorkload(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "workload_not_found")
		return
	}
	writeJSON(w, http.StatusOK, workload)
}

func (app *App) listNodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.cp.ListNodes())
}

func (app *App) listEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.cp.ListEvents())
}

func (app *App) schedulerTick(w http.ResponseWriter, r *http.Request) {
	results, err := app.cp.SchedulerTick()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scheduler_tick_failed")
		return
	}
	writeJSON(w, http.StatusOK, results)
}

type demoDataResponse struct {
	Action string `json:"action"`
	store.DemoDataSummary
}

func (app *App) seedDemoData(w http.ResponseWriter, r *http.Request) {
	summary, err := app.cp.SeedDemoData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "seed_demo_data_failed")
		return
	}
	writeJSON(w, http.StatusOK, demoDataResponse{Action: "seed", DemoDataSummary: summary})
}

func (app *App) clearDemoData(w http.ResponseWriter, r *http.Request) {
	summary, err := app.cp.ClearDemoData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "clear_demo_data_failed")
		return
	}
	writeJSON(w, http.StatusOK, demoDataResponse{Action: "clear", DemoDataSummary: summary})
}

func (app *App) failNode(w http.ResponseWriter, r *http.Request) {
	result, err := app.cp.FailNode(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *App) recoverNode(w http.ResponseWriter, r *http.Request) {
	result, err := app.cp.RecoverNode(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *App) preemptSpotNode(w http.ResponseWriter, r *http.Request) {
	result, err := app.cp.PreemptSpotNode(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *App) fleetSummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.cp.FleetSummary())
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
