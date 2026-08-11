//go:build linux && cgo

package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/jobdock/jobdock/internal/domain"
)

type nvmlDiscoverer struct{}

var nvmlMu sync.Mutex

func newGPUDiscoverer() GPUDiscoverer { return nvmlDiscoverer{} }

func (nvmlDiscoverer) Discover(ctx context.Context) ([]domain.GPU, domain.GPUDiscovery) {
	nvmlMu.Lock()
	defer nvmlMu.Unlock()
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

func (nvmlDiscoverer) Sample(ctx context.Context, uuids []string) (GPUUsage, error) {
	nvmlMu.Lock()
	defer nvmlMu.Unlock()
	if err := ctx.Err(); err != nil {
		return GPUUsage{}, err
	}
	if len(uuids) == 0 {
		return GPUUsage{}, nil
	}
	if result := nvml.Init(); result != nvml.SUCCESS {
		return GPUUsage{}, fmt.Errorf("initialize NVML: %s", nvml.ErrorString(result))
	}
	defer nvml.Shutdown()
	var usage GPUUsage
	for _, uuid := range uuids {
		device, result := nvml.DeviceGetHandleByUUID(uuid)
		if result != nvml.SUCCESS {
			return GPUUsage{}, fmt.Errorf("get GPU %s: %s", uuid, nvml.ErrorString(result))
		}
		utilization, result := device.GetUtilizationRates()
		if result != nvml.SUCCESS {
			return GPUUsage{}, fmt.Errorf("get GPU %s utilization: %s", uuid, nvml.ErrorString(result))
		}
		memory, result := device.GetMemoryInfo()
		if result != nvml.SUCCESS {
			return GPUUsage{}, fmt.Errorf("get GPU %s memory: %s", uuid, nvml.ErrorString(result))
		}
		usage.UtilizationBasisPoints += int64(utilization.Gpu) * 100
		usage.MemoryBytes += int64(memory.Used)
	}
	usage.UtilizationBasisPoints /= int64(len(uuids))
	return usage, nil
}
