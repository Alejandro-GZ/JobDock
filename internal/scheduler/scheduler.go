package scheduler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/capacity"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

type Scheduler struct {
	store *store.Store
	box   *secretbox.Box
}

type candidate struct {
	node      domain.Node
	gpuUUIDs  []string
	vramWaste int64
	ramWaste  int64
	cpuWaste  int64
}

func New(repository *store.Store, box *secretbox.Box) *Scheduler {
	return &Scheduler{store: repository, box: box}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_ = s.Schedule(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) Schedule(ctx context.Context) error {
	jobs, err := s.store.QueuedJobs(ctx)
	if err != nil || len(jobs) == 0 {
		return err
	}
	nodes, err := capacity.Snapshot(ctx, s.store)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		selected, code, message := selectNode(job.Spec, nodes)
		if selected == nil {
			_ = s.store.SetQueueReason(ctx, job.ID, code, message)
			continue
		}
		attemptID, assignmentID := ids.New(), ids.New()
		jobToken := ids.Token(32)
		encrypted, err := s.box.Encrypt([]byte(jobToken), []byte("assignment/"+assignmentID))
		if err != nil {
			return err
		}
		err = s.store.ReserveJob(ctx, job.ID, selected.node.ID, attemptID, assignmentID, auth.TokenHash(jobToken), encrypted, selected.gpuUUIDs)
		if err != nil {
			if err == store.ErrConflict {
				continue
			}
			return err
		}
		reserveInSnapshot(nodes, selected.node.ID, job.Spec.Resources, selected.gpuUUIDs)
	}
	return nil
}

func selectNode(spec domain.JobSpec, nodes []domain.Node) (*candidate, string, string) {
	var candidates []candidate
	var hasOnline, hasLabels, hasCPU, hasMemory, hasGPUModel bool
	for _, node := range nodes {
		if node.Status != domain.NodeOnline {
			continue
		}
		hasOnline = true
		if !labelsMatch(node.Labels, spec.NodeSelector) {
			continue
		}
		hasLabels = true
		cpuFree := node.CPUTotalMillis - node.CPUAllocatedMillis
		if cpuFree < spec.Resources.CPUMillis {
			continue
		}
		hasCPU = true
		memoryFree := node.MemoryTotalBytes - node.MemoryAllocatedBytes
		if memoryFree < spec.Resources.MemoryBytes {
			continue
		}
		hasMemory = true
		gpus := append([]domain.GPU(nil), node.GPUs...)
		sort.Slice(gpus, func(i, j int) bool {
			if gpus[i].VRAMBytes == gpus[j].VRAMBytes {
				return gpus[i].UUID < gpus[j].UUID
			}
			return gpus[i].VRAMBytes < gpus[j].VRAMBytes
		})
		var chosen []string
		var vramWaste int64
		for _, gpu := range gpus {
			if gpu.VRAMBytes >= spec.Resources.GPU.MinVRAMBytes {
				hasGPUModel = true
			}
			if !gpu.Allocated && gpu.VRAMBytes >= spec.Resources.GPU.MinVRAMBytes && len(chosen) < spec.Resources.GPU.Count {
				chosen = append(chosen, gpu.UUID)
				vramWaste += gpu.VRAMBytes - spec.Resources.GPU.MinVRAMBytes
			}
		}
		if len(chosen) != spec.Resources.GPU.Count {
			continue
		}
		candidates = append(candidates, candidate{node: node, gpuUUIDs: chosen, vramWaste: vramWaste, ramWaste: memoryFree - spec.Resources.MemoryBytes, cpuWaste: cpuFree - spec.Resources.CPUMillis})
	}
	if len(candidates) == 0 {
		switch {
		case !hasOnline:
			return nil, "NO_ONLINE_NODE", "Waiting: no execution node is online"
		case !hasLabels:
			return nil, "NODE_SELECTOR_MISMATCH", "Waiting: no online node matches the requested labels"
		case !hasCPU:
			return nil, "INSUFFICIENT_CPU", "Waiting: compatible nodes do not have enough unreserved CPU"
		case !hasMemory:
			return nil, "INSUFFICIENT_MEMORY", "Waiting: compatible nodes do not have enough unreserved memory"
		case spec.Resources.GPU.Count > 0 && !hasGPUModel:
			return nil, "NO_COMPATIBLE_GPU", fmt.Sprintf("Waiting: no node has %d GPU(s) with the requested VRAM", spec.Resources.GPU.Count)
		default:
			return nil, "RESOURCES_ALLOCATED", "Waiting: compatible resources exist but are currently allocated"
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.vramWaste != b.vramWaste {
			return a.vramWaste < b.vramWaste
		}
		if a.ramWaste != b.ramWaste {
			return a.ramWaste < b.ramWaste
		}
		if a.cpuWaste != b.cpuWaste {
			return a.cpuWaste < b.cpuWaste
		}
		return a.node.ID < b.node.ID
	})
	return &candidates[0], "", ""
}

func labelsMatch(labels, selector map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func reserveInSnapshot(nodes []domain.Node, nodeID string, resources domain.Resources, gpuUUIDs []string) {
	set := map[string]bool{}
	for _, id := range gpuUUIDs {
		set[id] = true
	}
	for i := range nodes {
		if nodes[i].ID != nodeID {
			continue
		}
		nodes[i].CPUAllocatedMillis += resources.CPUMillis
		nodes[i].MemoryAllocatedBytes += resources.MemoryBytes
		for j := range nodes[i].GPUs {
			if set[nodes[i].GPUs[j].UUID] {
				nodes[i].GPUs[j].Allocated = true
			}
		}
	}
}
