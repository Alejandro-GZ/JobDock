package agent

import (
	"context"

	"github.com/jobdock/jobdock/internal/domain"
)

type GPUDiscoverer interface {
	Discover(context.Context) ([]domain.GPU, domain.GPUDiscovery)
	Sample(context.Context, []string) (GPUUsage, error)
}

type GPUUsage struct {
	UtilizationBasisPoints int64
	MemoryBytes            int64
}

func resolveGPUInventory(ctx context.Context, mode string, discoverer GPUDiscoverer) ([]domain.GPU, domain.GPUDiscovery, bool) {
	if mode == "disabled" {
		return []domain.GPU{}, domain.GPUDiscovery{Status: "disabled", Message: "GPU discovery is disabled"}, false
	}
	gpus, discovery := discoverer.Discover(ctx)
	return gpus, discovery, mode == "required" && discovery.Status != "available"
}
