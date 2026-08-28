package agent

import (
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

func TestGPUTelemetryBufferCalculatesAndPrunesWindow(t *testing.T) {
	buffer := newGPUTelemetryBuffer()
	buffer.setDevices([]domain.GPU{{UUID: "GPU-1"}, {UUID: "GPU-2"}})
	if ids := buffer.deviceIDs(); len(ids) != 2 || ids[1] != "GPU-2" {
		t.Fatalf("unexpected device IDs: %#v", ids)
	}
	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	temperature := int64(51)
	buffer.add(map[string]GPUDeviceUsage{"GPU-1": {UtilizationBasisPoints: 1000, MemoryBytes: 100, SampledAt: base}})
	buffer.add(map[string]GPUDeviceUsage{"GPU-1": {UtilizationBasisPoints: 5000, MemoryBytes: 200, TemperatureCelsius: &temperature, SampledAt: base.Add(5 * time.Second)}})
	gpus := []domain.GPU{{UUID: "GPU-1"}}
	buffer.apply(gpus)
	if gpus[0].UtilizationAverageBasisPoints == nil || *gpus[0].UtilizationAverageBasisPoints != 3000 || gpus[0].UtilizationPeakBasisPoints == nil || *gpus[0].UtilizationPeakBasisPoints != 5000 || gpus[0].UtilizationSampleCount != 2 {
		t.Fatalf("unexpected summary: %#v", gpus[0])
	}
	buffer.add(map[string]GPUDeviceUsage{"GPU-1": {UtilizationBasisPoints: 9000, MemoryBytes: 300, SampledAt: base.Add(16 * time.Second)}})
	gpus = []domain.GPU{{UUID: "GPU-1"}}
	buffer.apply(gpus)
	if *gpus[0].UtilizationAverageBasisPoints != 9000 || gpus[0].UtilizationSampleCount != 1 || *gpus[0].MemoryUsedBytes != 300 {
		t.Fatalf("expired samples were not pruned: %#v", gpus[0])
	}
}

func TestGPUTelemetryBufferKeepsLastSummaryWhenSamplingStops(t *testing.T) {
	buffer := newGPUTelemetryBuffer()
	base := time.Now().UTC().Add(-time.Minute)
	buffer.add(map[string]GPUDeviceUsage{"GPU-1": {UtilizationBasisPoints: 4200, SampledAt: base}})
	gpus := []domain.GPU{{UUID: "GPU-1"}}
	buffer.apply(gpus)
	if gpus[0].UtilizationSampledAt == nil || !gpus[0].UtilizationSampledAt.Equal(base) || *gpus[0].UtilizationAverageBasisPoints != 4200 {
		t.Fatalf("last valid summary was not retained: %#v", gpus[0])
	}
}
