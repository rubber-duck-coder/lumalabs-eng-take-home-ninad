package events

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/scheduler"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

type Recorder struct {
	store store.Store
	now   func() time.Time
	seq   atomic.Uint64
}

func New(appStore store.Store, now func() time.Time) *Recorder {
	if now == nil {
		now = time.Now
	}
	return &Recorder{store: appStore, now: now}
}

func (r *Recorder) Record(eventType, actor, workloadID, nodeID, message string, metadata map[string]string) {
	now := r.now().UTC()
	_, _ = r.store.CreateEvent(domain.Event{
		ID:         r.nextID("event", now),
		Timestamp:  now,
		Type:       eventType,
		Actor:      actor,
		WorkloadID: workloadID,
		NodeID:     nodeID,
		Message:    message,
		Metadata:   metadata,
	})
}

func (r *Recorder) RecordScheduledResults(results []store.SchedulingResult) {
	for _, result := range results {
		r.RecordSchedulingEvent(result)
	}
}

func (r *Recorder) RecordSchedulingEvent(result store.SchedulingResult) {
	decision := result.Decision
	if decision.Outcome == "" {
		return
	}
	if decision.Outcome == scheduler.OutcomePlaced && decision.SelectedNode != nil {
		r.Record("workload_scheduled", "scheduler", result.Workload.ID, decision.SelectedNode.ID, decision.Reason, nil)
		return
	}
	r.Record("workload_queued", "scheduler", result.Workload.ID, "", decision.Reason, rejectedMetadata(decision.RejectedNodes))
}

func (r *Recorder) RecordAffectedWorkloads(eventType string, workloads []domain.Workload, nodeID string) {
	for _, workload := range workloads {
		r.Record(eventType, "system", workload.ID, nodeID, workload.StatusReason, nil)
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

func (r *Recorder) nextID(prefix string, now time.Time) string {
	seq := r.seq.Add(1)
	return prefix + "-" + strconv.FormatInt(now.UnixNano(), 36) + "-" + strconv.FormatUint(seq, 36)
}
