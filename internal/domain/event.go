package domain

import "time"

type Event struct {
	ID         string            `json:"id"`
	Timestamp  time.Time         `json:"timestamp"`
	Type       string            `json:"type"`
	Actor      string            `json:"actor"`
	WorkloadID string            `json:"workload_id,omitempty"`
	NodeID     string            `json:"node_id,omitempty"`
	Message    string            `json:"message"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
