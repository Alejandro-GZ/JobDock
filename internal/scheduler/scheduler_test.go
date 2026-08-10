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
