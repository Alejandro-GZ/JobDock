package agent

import (
	"context"
	"testing"

	"github.com/jobdock/jobdock/internal/domain"
)

type fakeGPUDiscoverer struct {
	gpus      []domain.GPU
	discovery domain.GPUDiscovery
	usage     GPUUsage
	sampleErr error
}

func (f fakeGPUDiscoverer) Sample(context.Context, []string) (GPUUsage, error) {
	return f.usage, f.sampleErr
}

func (f fakeGPUDiscoverer) SampleDevices(context.Context, []string) (map[string]GPUDeviceUsage, error) {
	return map[string]GPUDeviceUsage{}, f.sampleErr
}

func (f fakeGPUDiscoverer) Discover(context.Context) ([]domain.GPU, domain.GPUDiscovery) {
	return f.gpus, f.discovery
}

func TestResolveGPUInventoryModes(t *testing.T) {
	available := fakeGPUDiscoverer{gpus: []domain.GPU{{UUID: "GPU-1", Model: "Test", VRAMBytes: 8 << 30}}, discovery: domain.GPUDiscovery{Status: "available"}}
	gpus, discovery, degraded := resolveGPUInventory(context.Background(), "required", available)
	if len(gpus) != 1 || discovery.Status != "available" || degraded {
		t.Fatalf("unexpected available result: %#v %#v %v", gpus, discovery, degraded)
	}

	unavailable := fakeGPUDiscoverer{discovery: domain.GPUDiscovery{Status: "unavailable", ErrorCode: "NVML_UNAVAILABLE"}}
	_, _, degraded = resolveGPUInventory(context.Background(), "auto", unavailable)
	if degraded {
		t.Fatal("auto mode must not degrade a CPU-capable node")
	}
	_, _, degraded = resolveGPUInventory(context.Background(), "required", unavailable)
	if !degraded {
		t.Fatal("required mode must degrade a node without GPU discovery")
	}
	gpus, discovery, degraded = resolveGPUInventory(context.Background(), "disabled", available)
	if len(gpus) != 0 || discovery.Status != "disabled" || degraded {
		t.Fatalf("unexpected disabled result: %#v %#v %v", gpus, discovery, degraded)
	}
}
