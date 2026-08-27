package scheduler

import (
	"testing"

	"github.com/jobdock/jobdock/internal/domain"
)

func TestSelectNodeBestFitGPU(t *testing.T) {
	spec := domain.JobSpec{Resources: domain.Resources{CPUMillis: 1000, MemoryBytes: 1024, GPU: domain.GPURequest{Count: 1, MinVRAMBytes: 16}}}
	nodes := []domain.Node{
		{ID: "large", Status: domain.NodeOnline, CPUTotalMillis: 8000, MemoryTotalBytes: 8192, GPUs: []domain.GPU{{UUID: "80", VRAMBytes: 80}}},
		{ID: "small", Status: domain.NodeOnline, CPUTotalMillis: 8000, MemoryTotalBytes: 8192, GPUs: []domain.GPU{{UUID: "24", VRAMBytes: 24}}},
	}
	got, _, _ := selectNode(spec, nodes)
	if got == nil || got.node.ID != "small" {
		t.Fatalf("expected small best-fit node, got %#v", got)
	}
}

func TestSelectNodeExplainsShortage(t *testing.T) {
	spec := domain.JobSpec{Resources: domain.Resources{CPUMillis: 1000, MemoryBytes: 1024}}
	_, code, _ := selectNode(spec, nil)
	if code != "NO_ONLINE_NODE" {
		t.Fatalf("unexpected reason %s", code)
	}
}

func TestSelectNodeUsesExplicitGPUAndCPUPackage(t *testing.T) {
	spec := domain.JobSpec{TargetNodeID: "node-a", Resources: domain.Resources{CPUMillis: 1500, CPUPackageID: "1", MemoryBytes: 1024, GPU: domain.GPURequest{Count: 1, UUIDs: []string{"GPU-B"}}}}
	nodes := []domain.Node{{ID: "node-a", Status: domain.NodeOnline, CPUTotalMillis: 8000, MemoryTotalBytes: 8192, Capabilities: []string{"cpu_package_affinity"}, CPUPackages: []domain.CPUPackage{{ID: "1", LogicalCPUs: []int{2, 6}, TotalMillis: 2000}}, GPUs: []domain.GPU{{UUID: "GPU-A"}, {UUID: "GPU-B"}}}, {ID: "node-b", Status: domain.NodeOnline, CPUTotalMillis: 8000, MemoryTotalBytes: 8192, GPUs: []domain.GPU{{UUID: "GPU-B"}}}}
	selected, code, _ := selectNode(spec, nodes)
	if code != "" || selected == nil || selected.node.ID != "node-a" || selected.cpuSet != "2,6" || len(selected.gpuUUIDs) != 1 || selected.gpuUUIDs[0] != "GPU-B" {
		t.Fatalf("unexpected selection: %#v %s", selected, code)
	}
}

func TestSelectNodeWaitsForExplicitHardware(t *testing.T) {
	base := domain.Node{ID: "node-a", Status: domain.NodeOnline, CPUTotalMillis: 8000, MemoryTotalBytes: 8192, Capabilities: []string{"cpu_package_affinity"}, CPUPackages: []domain.CPUPackage{{ID: "0", LogicalCPUs: []int{0}, TotalMillis: 1000}}, GPUs: []domain.GPU{{UUID: "GPU-A", Allocated: true}}}
	spec := domain.JobSpec{TargetNodeID: "node-a", Resources: domain.Resources{CPUMillis: 500, MemoryBytes: 1024, GPU: domain.GPURequest{Count: 1, UUIDs: []string{"GPU-A"}}}}
	if _, code, _ := selectNode(spec, []domain.Node{base}); code != "REQUESTED_GPU_ALLOCATED" {
		t.Fatalf("unexpected GPU reason %s", code)
	}
	spec.Resources.GPU = domain.GPURequest{}
	spec.Resources.CPUPackageID = "missing"
	if _, code, _ := selectNode(spec, []domain.Node{base}); code != "REQUESTED_CPU_PACKAGE_NOT_FOUND" {
		t.Fatalf("unexpected CPU reason %s", code)
	}
}

func BenchmarkSelectNodeFiftyNodes(b *testing.B) {
	nodes := make([]domain.Node, 50)
	for index := range nodes {
		nodes[index] = domain.Node{ID: string(rune('a'+index%26)) + string(rune('a'+index/26)), Status: domain.NodeOnline, CPUTotalMillis: 64000, MemoryTotalBytes: 256 << 30, GPUs: []domain.GPU{{UUID: "gpu-a", VRAMBytes: 24 << 30}, {UUID: "gpu-b", VRAMBytes: 48 << 30}}}
	}
	spec := domain.JobSpec{Resources: domain.Resources{CPUMillis: 4000, MemoryBytes: 16 << 30, GPU: domain.GPURequest{Count: 1, MinVRAMBytes: 20 << 30}}}
	b.ResetTimer()
	for range b.N {
		selected, _, _ := selectNode(spec, nodes)
		if selected == nil {
			b.Fatal("no placement")
		}
	}
}
