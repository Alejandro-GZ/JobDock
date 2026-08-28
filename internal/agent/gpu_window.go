package agent

import (
	"sync"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

const gpuTelemetryWindow = 10 * time.Second

type gpuWindowSample struct {
	utilization int64
	memory      int64
	temperature *int64
	capturedAt  time.Time
}

type gpuWindowSummary struct {
	average int64
	peak    int64
	count   int
	latest  gpuWindowSample
}

type gpuTelemetryBuffer struct {
	mu        sync.Mutex
	devices   []string
	samples   map[string][]gpuWindowSample
	summaries map[string]gpuWindowSummary
}

func newGPUTelemetryBuffer() *gpuTelemetryBuffer {
	return &gpuTelemetryBuffer{samples: map[string][]gpuWindowSample{}, summaries: map[string]gpuWindowSummary{}}
}

func (b *gpuTelemetryBuffer) setDevices(gpus []domain.GPU) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.devices = b.devices[:0]
	for _, gpu := range gpus {
		b.devices = append(b.devices, gpu.UUID)
	}
}

func (b *gpuTelemetryBuffer) deviceIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.devices...)
}

func (b *gpuTelemetryBuffer) add(values map[string]GPUDeviceUsage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for uuid, value := range values {
		capturedAt := value.SampledAt.UTC()
		entry := gpuWindowSample{utilization: value.UtilizationBasisPoints, memory: value.MemoryBytes, temperature: value.TemperatureCelsius, capturedAt: capturedAt}
		cutoff := capturedAt.Add(-gpuTelemetryWindow)
		window := append(b.samples[uuid], entry)
		first := 0
		for first < len(window) && window[first].capturedAt.Before(cutoff) {
			first++
		}
		window = append([]gpuWindowSample(nil), window[first:]...)
		b.samples[uuid] = window
		var total, peak int64
		for _, sample := range window {
			total += sample.utilization
			if sample.utilization > peak {
				peak = sample.utilization
			}
		}
		b.summaries[uuid] = gpuWindowSummary{average: total / int64(len(window)), peak: peak, count: len(window), latest: entry}
	}
}

func (b *gpuTelemetryBuffer) apply(gpus []domain.GPU) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for index := range gpus {
		summary, ok := b.summaries[gpus[index].UUID]
		if !ok {
			continue
		}
		instant, average, peak := summary.latest.utilization, summary.average, summary.peak
		memory, sampledAt := summary.latest.memory, summary.latest.capturedAt
		if gpus[index].UtilizationBasisPoints == nil {
			gpus[index].UtilizationBasisPoints = &instant
		}
		gpus[index].UtilizationAverageBasisPoints = &average
		gpus[index].UtilizationPeakBasisPoints = &peak
		gpus[index].UtilizationSampledAt = &sampledAt
		gpus[index].UtilizationWindowSeconds = int(gpuTelemetryWindow / time.Second)
		gpus[index].UtilizationSampleCount = summary.count
		if gpus[index].MemoryUsedBytes == nil {
			gpus[index].MemoryUsedBytes = &memory
		}
		if gpus[index].TemperatureCelsius == nil {
			gpus[index].TemperatureCelsius = summary.latest.temperature
		}
	}
}
