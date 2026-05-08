package store

import (
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
)

type Store interface {
	SeedDemoData() (DemoDataSummary, error)
	Clear() (DemoDataSummary, error)
	CreateWorkload(workload domain.Workload) (domain.Workload, error)
	GetWorkload(id string) (domain.Workload, bool)
	ListWorkloads() []domain.Workload
	UpdateWorkload(id string, fn func(*domain.Workload) error) (domain.Workload, error)
	CompleteExpiredWorkloads(now time.Time) ([]domain.Workload, error)
	ScheduleWorkload(id string, now time.Time) (SchedulingResult, error)
	SchedulePendingWorkloads(now time.Time) ([]SchedulingResult, error)
	FailNode(id string, now time.Time) (DisruptionResult, error)
	RecoverNode(id string, now time.Time) (DisruptionResult, error)
	PreemptSpotNode(id string, now time.Time) (DisruptionResult, error)
	CreateNode(node domain.Node) (domain.Node, error)
	GetNode(id string) (domain.Node, bool)
	ListNodes() []domain.Node
	UpdateNode(id string, fn func(*domain.Node) error) (domain.Node, error)
	CreateEvent(event domain.Event) (domain.Event, error)
	GetEvent(id string) (domain.Event, bool)
	ListEvents() []domain.Event
	UpdateEvent(id string, fn func(*domain.Event) error) (domain.Event, error)
}

type DemoDataSummary struct {
	Nodes     int `json:"nodes"`
	Workloads int `json:"workloads"`
	Events    int `json:"events"`
}

var _ Store = (*MemoryStore)(nil)
