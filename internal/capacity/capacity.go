package capacity

import (
	"context"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/store"
)

func Snapshot(ctx context.Context, repository *store.Store) ([]domain.Node, error) {
	nodes, err := repository.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	jobs, err := repository.ListJobs(ctx, false)
	if err != nil {
		return nil, err
	}
	gpuAllocations, err := repository.AllocatedGPUUUIDs(ctx)
	if err != nil {
		return nil, err
	}
	Account(nodes, jobs, gpuAllocations)
	return nodes, nil
}

func Account(nodes []domain.Node, jobs []domain.Job, gpuAllocations map[string]map[string]bool) {
	byID := make(map[string]*domain.Node, len(nodes))
	for index := range nodes {
		byID[nodes[index].ID] = &nodes[index]
		for gpuIndex := range nodes[index].GPUs {
			nodes[index].GPUs[gpuIndex].Allocated = gpuAllocations[nodes[index].ID][nodes[index].GPUs[gpuIndex].UUID]
		}
	}
	for _, job := range jobs {
		if !domain.IsActive(job.Status) || job.AssignedNodeID == "" {
			continue
		}
		node := byID[job.AssignedNodeID]
		if node == nil {
			continue
		}
		node.CPUAllocatedMillis += job.Spec.Resources.CPUMillis
		if job.Spec.Resources.CPUPackageID != "" {
			for index := range node.CPUPackages {
				if node.CPUPackages[index].ID == job.Spec.Resources.CPUPackageID {
					node.CPUPackages[index].AllocatedMillis += job.Spec.Resources.CPUMillis
				}
			}
		}
		node.MemoryAllocatedBytes += job.Spec.Resources.MemoryBytes
	}
}
