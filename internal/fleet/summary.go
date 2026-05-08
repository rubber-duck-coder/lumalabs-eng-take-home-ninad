package fleet

import "github.com/ninadsindu/luma-gpu-control-plane/internal/domain"

type Summary struct {
	TotalGPUs        int                          `json:"total_gpus"`
	AllocatedGPUs    int                          `json:"allocated_gpus"`
	AvailableGPUs    int                          `json:"available_gpus"`
	FailedGPUs       int                          `json:"failed_gpus"`
	Utilization      float64                      `json:"utilization_percent"`
	GPUTypes         map[string]map[string]int    `json:"gpu_types"`
	WorkloadsByState map[domain.WorkloadState]int `json:"workloads_by_state"`
}

func Build(nodes []domain.Node, workloads []domain.Workload) Summary {
	var total, allocated, available, failed int
	byGPU := map[string]map[string]int{}
	for _, node := range nodes {
		total += node.TotalGPUs
		allocated += node.AllocatedGPUs
		if _, ok := byGPU[node.GPUType]; !ok {
			byGPU[node.GPUType] = map[string]int{"total": 0, "allocated": 0, "available": 0, "failed": 0}
		}
		byGPU[node.GPUType]["total"] += node.TotalGPUs
		byGPU[node.GPUType]["allocated"] += node.AllocatedGPUs
		if node.Health == domain.NodeHealthFailed {
			failed += node.TotalGPUs
			byGPU[node.GPUType]["failed"] += node.TotalGPUs
			continue
		}
		free := node.TotalGPUs - node.AllocatedGPUs
		if free > 0 {
			available += free
			byGPU[node.GPUType]["available"] += free
		}
	}

	byState := map[domain.WorkloadState]int{}
	for _, workload := range workloads {
		byState[workload.State]++
	}

	utilization := 0.0
	if total > 0 {
		utilization = float64(allocated) / float64(total) * 100
	}

	return Summary{
		TotalGPUs:        total,
		AllocatedGPUs:    allocated,
		AvailableGPUs:    available,
		FailedGPUs:       failed,
		Utilization:      utilization,
		GPUTypes:         byGPU,
		WorkloadsByState: byState,
	}
}
