package domain

import "time"

type CapacityClass string

const (
	CapacityClassOnDemand CapacityClass = "on_demand"
	CapacityClassSpot     CapacityClass = "spot"
)

type NodeHealth string

const (
	NodeHealthHealthy    NodeHealth = "healthy"
	NodeHealthFailed     NodeHealth = "failed"
	NodeHealthRecovering NodeHealth = "recovering"
)

type Node struct {
	ID                 string        `json:"id"`
	GPUType            string        `json:"gpu_type"`
	TotalGPUs          int           `json:"total_gpus"`
	AllocatedGPUs      int           `json:"allocated_gpus"`
	Region             string        `json:"region"`
	DataCenter         string        `json:"data_center"`
	Zone               string        `json:"zone"`
	Provider           string        `json:"provider"`
	CapacityClass      CapacityClass `json:"capacity_class"`
	Health             NodeHealth    `json:"health"`
	RunningWorkloadIDs []string      `json:"running_workload_ids,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

func (n Node) FreeGPUs() int {
	free := n.TotalGPUs - n.AllocatedGPUs
	if free < 0 {
		return 0
	}
	return free
}

func (n Node) CanFit(gpuCount int) bool {
	return gpuCount > 0 && n.Health == NodeHealthHealthy && n.FreeGPUs() >= gpuCount
}

func (n Node) IsSpot() bool {
	return n.CapacityClass == CapacityClassSpot
}
