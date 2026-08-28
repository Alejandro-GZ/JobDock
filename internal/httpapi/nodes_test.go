package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestNodeDetailAttributesReservationsAndScopesTelemetry(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := createSeriesUser(t, repository, "node-owner")
	other := createSeriesUser(t, repository, "node-other")
	admin := createSeriesUser(t, repository, "node-admin")
	if _, err = repository.DB().ExecContext(ctx, `UPDATE users SET role='admin' WHERE id=?`, admin.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	utilization, average, peak, used, temperature := int64(7200), int64(4800), int64(8300), int64(2<<30), int64(61)
	node := domain.Node{ID: ids.New(), Name: "inventory-node", Status: domain.NodeOnline, AgentVersion: "test", ProtocolVersion: 1, Architecture: "amd64", DockerVersion: "29", CPUTotalMillis: 8000, MemoryTotalBytes: 16 << 30, WorkspaceTotalBytes: 100 << 30, WorkspaceFreeBytes: 80 << 30, Labels: map[string]string{}, Capabilities: []string{"host_inventory_v1", "cpu_package_affinity", "gpu_window_telemetry_v1"}, CPUPackages: []domain.CPUPackage{{ID: "0", Vendor: "GenuineIntel", Model: "Test CPU", PhysicalCores: 4, LogicalCPUs: []int{0, 1, 2, 3, 4, 5, 6, 7}, TotalMillis: 8000}}, GPUs: []domain.GPU{{UUID: "GPU-1", Model: "Test GPU", VRAMBytes: 8 << 30, PCIBusID: "0000:01:00.0", DriverVersion: "580", ComputeCapability: "8.9", UtilizationBasisPoints: &utilization, UtilizationAverageBasisPoints: &average, UtilizationPeakBasisPoints: &peak, UtilizationSampledAt: &now, UtilizationWindowSeconds: 10, UtilizationSampleCount: 10, MemoryUsedBytes: &used, TemperatureCelsius: &temperature}, {UUID: "GPU-2", Model: "Test GPU", VRAMBytes: 8 << 30, UtilizationBasisPoints: &utilization, UtilizationAverageBasisPoints: &average, UtilizationPeakBasisPoints: &peak, UtilizationSampledAt: &now, UtilizationWindowSeconds: 10, UtilizationSampleCount: 10, MemoryUsedBytes: &used, TemperatureCelsius: &temperature}}, GPUDiscovery: domain.GPUDiscovery{Status: "available"}, System: domain.NodeSystemInfo{Hostname: "worker-1", OperatingSystem: "Linux", KernelVersion: "6.8", Architecture: "amd64"}, Runtime: domain.NodeRuntimeInfo{DockerVersion: "29", StorageDriver: "overlay2", CgroupVersion: "2"}, LastHeartbeat: now, CreatedAt: now}
	if err = repository.UpsertNode(ctx, node, "node-detail-credential"); err != nil {
		t.Fatal(err)
	}
	create := func(user domain.User, name string, gpu []string, cpu int64) domain.Job {
		job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: name, Resources: domain.Resources{CPUMillis: 1000, CPUPackageID: "0", MemoryBytes: 2 << 30, GPU: domain.GPURequest{Count: len(gpu), UUIDs: gpu}}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: now}
		if createErr := repository.CreateJob(ctx, job); createErr != nil {
			t.Fatal(createErr)
		}
		attemptID := ids.New()
		if createErr := repository.ReserveJobWithAffinity(ctx, job.ID, node.ID, attemptID, ids.New(), ids.New(), []byte("cipher"), gpu, "0", "0-3"); createErr != nil {
			t.Fatal(createErr)
		}
		if createErr := repository.AppendResourceSample(ctx, domain.ResourceSample{JobID: job.ID, AttemptID: attemptID, CapturedAt: now, CPUMillis: cpu, MemoryBytes: cpu * 1024}); createErr != nil {
			t.Fatal(createErr)
		}
		return job
	}
	owned := create(owner, "owned", []string{"GPU-1"}, 400)
	foreign := create(other, "foreign", []string{"GPU-2"}, 800)
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{4}, 32))
	api := New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	memberClient := loginSeriesUser(t, server.URL, owner.Username)
	var member nodeDetail
	getSeriesJSON(t, memberClient, server.URL+"/api/v1/nodes/"+node.ID, &member)
	if member.Node.System.Hostname != "worker-1" || member.Node.Runtime.StorageDriver != "overlay2" || member.Usage.CPUObservedMillis != 1200 || len(member.Allocations) != 2 {
		t.Fatalf("unexpected node detail: %#v", member)
	}
	byID := map[string]nodeAllocation{}
	for _, item := range member.Allocations {
		byID[item.JobID] = item
	}
	if !byID[owned.ID].CanOpen || byID[owned.ID].ObservedCPUMillis == nil || byID[owned.ID].TelemetryStatus != "fresh" {
		t.Fatalf("owned allocation not visible: %#v", byID[owned.ID])
	}
	if byID[foreign.ID].CanOpen || byID[foreign.ID].TelemetryStatus != "restricted" || byID[foreign.ID].ObservedCPUMillis != nil {
		t.Fatalf("foreign allocation leaked telemetry: %#v", byID[foreign.ID])
	}
	if member.Node.GPUs[0].AllocatedJobID != owned.ID || member.Node.GPUs[0].UtilizationBasisPoints == nil || member.Node.GPUs[0].UtilizationAverageBasisPoints == nil || *member.Node.GPUs[0].UtilizationAverageBasisPoints != average || member.Node.GPUs[0].UtilizationSampleCount != 10 {
		t.Fatalf("GPU allocation was not attributed: %#v", member.Node.GPUs[0])
	}
	if member.Node.GPUs[1].AllocatedJobID != foreign.ID || member.Node.GPUs[1].UtilizationBasisPoints != nil || member.Node.GPUs[1].UtilizationAverageBasisPoints != nil || member.Node.GPUs[1].UtilizationPeakBasisPoints != nil || member.Node.GPUs[1].UtilizationSampledAt != nil || member.Node.GPUs[1].UtilizationSampleCount != 0 {
		t.Fatalf("foreign GPU telemetry leaked: %#v", member.Node.GPUs[1])
	}
	adminClient := loginSeriesUser(t, server.URL, admin.Username)
	var adminDetail nodeDetail
	getSeriesJSON(t, adminClient, server.URL+"/api/v1/nodes/"+node.ID, &adminDetail)
	for _, item := range adminDetail.Allocations {
		if !item.CanOpen || item.ObservedCPUMillis == nil {
			t.Fatalf("admin allocation should be fully visible: %#v", item)
		}
	}
}
