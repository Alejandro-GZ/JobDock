//go:build linux && cgo

package agent

import (
	"context"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/jobdock/jobdock/internal/domain"
)

type nvmlDiscoverer struct{}

func newGPUDiscoverer() GPUDiscoverer { return nvmlDiscoverer{} }

func (nvmlDiscoverer) Discover(ctx context.Context) ([]domain.GPU, domain.GPUDiscovery) {
	if err := ctx.Err(); err != nil {
		return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "DISCOVERY_FAILED", Message: err.Error()}
	}
	if result := nvml.Init(); result != nvml.SUCCESS {
		return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "NVML_UNAVAILABLE", Message: nvml.ErrorString(result)}
	}
	defer nvml.Shutdown()
	count, result := nvml.DeviceGetCount()
	if result != nvml.SUCCESS {
		return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "DISCOVERY_FAILED", Message: nvml.ErrorString(result)}
	}
	if count == 0 {
		return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "NO_GPU_FOUND", Message: "NVML reported no NVIDIA GPUs"}
	}
	gpus := make([]domain.GPU, 0, count)
	for index := 0; index < count; index++ {
		device, code := nvml.DeviceGetHandleByIndex(index)
		if code != nvml.SUCCESS {
			return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "DISCOVERY_FAILED", Message: fmt.Sprintf("get GPU %d: %s", index, nvml.ErrorString(code))}
		}
		uuid, code := device.GetUUID()
		if code != nvml.SUCCESS {
			return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "DISCOVERY_FAILED", Message: fmt.Sprintf("get GPU %d UUID: %s", index, nvml.ErrorString(code))}
		}
		name, code := device.GetName()
		if code != nvml.SUCCESS {
			return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "DISCOVERY_FAILED", Message: fmt.Sprintf("get GPU %s name: %s", uuid, nvml.ErrorString(code))}
		}
		memory, code := device.GetMemoryInfo()
		if code != nvml.SUCCESS {
			return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "DISCOVERY_FAILED", Message: fmt.Sprintf("get GPU %s memory: %s", uuid, nvml.ErrorString(code))}
		}
		gpus = append(gpus, domain.GPU{UUID: uuid, Model: name, VRAMBytes: int64(memory.Total)})
	}
	return gpus, domain.GPUDiscovery{Status: "available", Message: fmt.Sprintf("Discovered %d NVIDIA GPU(s) through NVML", len(gpus))}
}
