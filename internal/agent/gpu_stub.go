//go:build !linux || !cgo

package agent

import (
	"context"
	"errors"
	"runtime"

	"github.com/jobdock/jobdock/internal/domain"
)

type unsupportedGPUDiscoverer struct{}

func newGPUDiscoverer() GPUDiscoverer { return unsupportedGPUDiscoverer{} }

func (unsupportedGPUDiscoverer) Discover(context.Context) ([]domain.GPU, domain.GPUDiscovery) {
	return []domain.GPU{}, domain.GPUDiscovery{Status: "unavailable", ErrorCode: "NVML_UNAVAILABLE", Message: "NVML discovery is unavailable on " + runtime.GOOS}
}

func (unsupportedGPUDiscoverer) Sample(context.Context, []string) (GPUUsage, error) {
	return GPUUsage{}, errors.New("NVML sampling is unavailable")
}

func (unsupportedGPUDiscoverer) SampleDevices(context.Context, []string) (map[string]GPUDeviceUsage, error) {
	return nil, errors.New("NVML sampling is unavailable")
}
