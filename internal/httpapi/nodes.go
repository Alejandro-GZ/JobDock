package httpapi

import (
	"net/http"
	"time"

	"github.com/jobdock/jobdock/internal/capacity"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/store"
)

type nodeUsage struct {
	CPUObservedMillis         int64      `json:"cpu_observed_millis"`
	MemoryObservedBytes       int64      `json:"memory_observed_bytes"`
	GPUMemoryObservedBytes    int64      `json:"gpu_memory_observed_bytes"`
	GPUUtilizationBasisPoints *int64     `json:"gpu_utilization_basis_points,omitempty"`
	LatestTelemetryAt         *time.Time `json:"latest_telemetry_at,omitempty"`
	ObservedAllocationCount   int        `json:"observed_allocation_count"`
}

type nodeAllocation struct {
	JobID                             string           `json:"job_id"`
	JobName                           string           `json:"job_name"`
	OwnerID                           string           `json:"owner_id,omitempty"`
	AttemptID                         string           `json:"attempt_id,omitempty"`
	Status                            domain.JobStatus `json:"status"`
	ReservedCPUMillis                 int64            `json:"reserved_cpu_millis"`
	ReservedMemoryBytes               int64            `json:"reserved_memory_bytes"`
	CPUPackageID                      string           `json:"cpu_package_id,omitempty"`
	CPUSet                            string           `json:"cpu_set,omitempty"`
	GPUUUIDs                          []string         `json:"gpu_uuids"`
	ObservedCPUMillis                 *int64           `json:"observed_cpu_millis,omitempty"`
	ObservedMemoryBytes               *int64           `json:"observed_memory_bytes,omitempty"`
	ObservedGPUUtilizationBasisPoints *int64           `json:"observed_gpu_utilization_basis_points,omitempty"`
	ObservedGPUMemoryBytes            *int64           `json:"observed_gpu_memory_bytes,omitempty"`
	TelemetryCapturedAt               *time.Time       `json:"telemetry_captured_at,omitempty"`
	TelemetryStatus                   string           `json:"telemetry_status"`
	CanOpen                           bool             `json:"can_open"`
}

type nodeDetail struct {
	Node        domain.Node      `json:"node"`
	Usage       nodeUsage        `json:"usage"`
	Allocations []nodeAllocation `json:"allocations"`
}

func (a *API) getNodeDetail(w http.ResponseWriter, r *http.Request) {
	nodes, err := capacity.Snapshot(r.Context(), a.store)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var node *domain.Node
	for index := range nodes {
		if nodes[index].ID == r.PathValue("id") {
			node = &nodes[index]
			break
		}
	}
	if node == nil {
		writeProblem(w, http.StatusNotFound, "node_not_found", "Node was not found")
		return
	}
	records, err := a.store.ActiveNodeAssignments(r.Context(), node.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	refs := make([]store.JobAttemptRef, 0, len(records))
	for _, record := range records {
		if record.Job.AttemptID != "" {
			refs = append(refs, store.JobAttemptRef{JobID: record.Job.ID, AttemptID: record.Job.AttemptID})
		}
	}
	summaries, _, err := a.store.ResourceSummaries(r.Context(), refs, 1, nil)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	user := currentUser(r)
	detail := nodeDetail{Node: *node, Allocations: make([]nodeAllocation, 0, len(records))}
	gpuOwners := make(map[string]nodeAllocation)
	var gpuUtilizationTotal int64
	var gpuUtilizationCount int64
	for _, record := range records {
		visible := user.Role == domain.RoleAdmin || record.Job.OwnerID == user.ID
		item := nodeAllocation{JobID: record.Job.ID, JobName: record.Job.Spec.Name, Status: record.Job.Status, ReservedCPUMillis: record.Job.Spec.Resources.CPUMillis, ReservedMemoryBytes: record.Job.Spec.Resources.MemoryBytes, CPUPackageID: record.Job.Spec.Resources.CPUPackageID, CPUSet: record.CPUSet, GPUUUIDs: record.GPUUUIDs, TelemetryStatus: "unavailable", CanOpen: visible}
		if item.GPUUUIDs == nil {
			item.GPUUUIDs = []string{}
		}
		if visible {
			item.OwnerID = record.Job.OwnerID
			item.AttemptID = record.Job.AttemptID
			if samples := summaries[record.Job.ID]; len(samples) > 0 {
				sample := samples[len(samples)-1]
				item.ObservedCPUMillis = &sample.CPUMillis
				item.ObservedMemoryBytes = &sample.MemoryBytes
				item.ObservedGPUUtilizationBasisPoints = sample.GPUUtilizationBasisPoints
				item.ObservedGPUMemoryBytes = sample.GPUMemoryBytes
				item.TelemetryCapturedAt = &sample.CapturedAt
				item.TelemetryStatus = telemetryFreshness(sample.CapturedAt)
			}
		} else {
			item.TelemetryStatus = "restricted"
		}
		if samples := summaries[record.Job.ID]; len(samples) > 0 {
			sample := samples[len(samples)-1]
			detail.Usage.CPUObservedMillis += sample.CPUMillis
			detail.Usage.MemoryObservedBytes += sample.MemoryBytes
			if sample.GPUMemoryBytes != nil {
				detail.Usage.GPUMemoryObservedBytes += *sample.GPUMemoryBytes
			}
			if sample.GPUUtilizationBasisPoints != nil {
				gpuUtilizationTotal += *sample.GPUUtilizationBasisPoints
				gpuUtilizationCount++
			}
			detail.Usage.ObservedAllocationCount++
			if detail.Usage.LatestTelemetryAt == nil || sample.CapturedAt.After(*detail.Usage.LatestTelemetryAt) {
				captured := sample.CapturedAt
				detail.Usage.LatestTelemetryAt = &captured
			}
		}
		for _, uuid := range item.GPUUUIDs {
			gpuOwners[uuid] = item
		}
		detail.Allocations = append(detail.Allocations, item)
	}
	if gpuUtilizationCount > 0 {
		value := gpuUtilizationTotal / gpuUtilizationCount
		detail.Usage.GPUUtilizationBasisPoints = &value
	}
	for index := range detail.Node.GPUs {
		owner, allocated := gpuOwners[detail.Node.GPUs[index].UUID]
		if !allocated {
			continue
		}
		detail.Node.GPUs[index].Allocated = true
		detail.Node.GPUs[index].AllocatedJobID = owner.JobID
		if !owner.CanOpen {
			detail.Node.GPUs[index].UtilizationBasisPoints = nil
			detail.Node.GPUs[index].UtilizationAverageBasisPoints = nil
			detail.Node.GPUs[index].UtilizationPeakBasisPoints = nil
			detail.Node.GPUs[index].UtilizationSampledAt = nil
			detail.Node.GPUs[index].UtilizationWindowSeconds = 0
			detail.Node.GPUs[index].UtilizationSampleCount = 0
			detail.Node.GPUs[index].MemoryUsedBytes = nil
			detail.Node.GPUs[index].TemperatureCelsius = nil
		}
	}
	writeJSON(w, http.StatusOK, detail)
}

func telemetryFreshness(captured time.Time) string {
	if time.Since(captured) > 30*time.Second {
		return "stale"
	}
	return "fresh"
}
