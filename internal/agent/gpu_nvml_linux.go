//go:build linux && cgo

package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	driverVersion, driverResult := nvml.SystemGetDriverVersion()
	if driverResult != nvml.SUCCESS {
		driverVersion = ""
	}
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
		gpu := domain.GPU{UUID: uuid, Model: name, VRAMBytes: int64(memory.Total), DriverVersion: driverVersion}
		used := int64(memory.Used)
		gpu.MemoryUsedBytes = &used
		if utilization, utilizationCode := device.GetUtilizationRates(); utilizationCode == nvml.SUCCESS {
			value := int64(utilization.Gpu) * 100
			gpu.UtilizationBasisPoints = &value
		}
		if temperature, temperatureCode := device.GetTemperature(nvml.TEMPERATURE_GPU); temperatureCode == nvml.SUCCESS {
			value := int64(temperature)
			gpu.TemperatureCelsius = &value
		}
		if pci, pciCode := device.GetPciInfo(); pciCode == nvml.SUCCESS {
			gpu.PCIBusID = nvmlString(pci.BusId[:])
		}
		if major, minor, capabilityCode := device.GetCudaComputeCapability(); capabilityCode == nvml.SUCCESS {
			gpu.ComputeCapability = fmt.Sprintf("%d.%d", major, minor)
		}
		gpus = append(gpus, gpu)
	}
	return gpus, domain.GPUDiscovery{Status: "available", Message: fmt.Sprintf("Discovered %d NVIDIA GPU(s) through NVML", len(gpus))}
}

func nvmlString(value []int8) string {
	bytes := make([]byte, 0, len(value))
	for _, character := range value {
		if character == 0 {
			break
		}
		bytes = append(bytes, byte(character))
	}
	return string(bytes)
}

func (nvmlDiscoverer) Sample(ctx context.Context, uuids []string) (GPUUsage, error) {
	nvmlMu.Lock()
	defer nvmlMu.Unlock()
	samples, err := sampleNVMLDevices(ctx, uuids)
	if err != nil {
		return GPUUsage{}, err
	}
	var usage GPUUsage
	for _, uuid := range uuids {
		sample := samples[uuid]
		usage.UtilizationBasisPoints += sample.UtilizationBasisPoints
		usage.MemoryBytes += sample.MemoryBytes
	}
	if len(uuids) > 0 {
		usage.UtilizationBasisPoints /= int64(len(uuids))
	}
	return usage, nil
}

func (nvmlDiscoverer) SampleDevices(ctx context.Context, uuids []string) (map[string]GPUDeviceUsage, error) {
	nvmlMu.Lock()
	defer nvmlMu.Unlock()
	return sampleNVMLDevices(ctx, uuids)
}

func sampleNVMLDevices(ctx context.Context, uuids []string) (map[string]GPUDeviceUsage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(uuids) == 0 {
		return map[string]GPUDeviceUsage{}, nil
	}
	if result := nvml.Init(); result != nvml.SUCCESS {
		return nil, fmt.Errorf("initialize NVML: %s", nvml.ErrorString(result))
	}
	defer nvml.Shutdown()
	sampledAt := time.Now().UTC()
	samples := make(map[string]GPUDeviceUsage, len(uuids))
	for _, uuid := range uuids {
		device, result := nvml.DeviceGetHandleByUUID(uuid)
		if result != nvml.SUCCESS {
			return nil, fmt.Errorf("get GPU %s: %s", uuid, nvml.ErrorString(result))
		}
		utilization, result := device.GetUtilizationRates()
		if result != nvml.SUCCESS {
			return nil, fmt.Errorf("get GPU %s utilization: %s", uuid, nvml.ErrorString(result))
		}
		memory, result := device.GetMemoryInfo()
		if result != nvml.SUCCESS {
			return nil, fmt.Errorf("get GPU %s memory: %s", uuid, nvml.ErrorString(result))
		}
		sample := GPUDeviceUsage{UtilizationBasisPoints: int64(utilization.Gpu) * 100, MemoryBytes: int64(memory.Used), SampledAt: sampledAt}
		if temperature, code := device.GetTemperature(nvml.TEMPERATURE_GPU); code == nvml.SUCCESS {
			value := int64(temperature)
			sample.TemperatureCelsius = &value
		}
		samples[uuid] = sample
	}
	return samples, nil
}
