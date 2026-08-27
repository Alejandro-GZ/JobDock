package dockerengine

import "testing"

func TestCreateRequestCombinesQuotaCPUSetAndGPUs(t *testing.T) {
	body := createRequest(CreateOptions{CPUMillis: 1500, CPUSet: "2,6", MemoryBytes: 2048, GPUUUIDs: []string{"GPU-A", "GPU-B"}})
	if body.HostConfig.NanoCPUs != 1_500_000_000 || body.HostConfig.CpusetCpus != "2,6" || body.HostConfig.Memory != 2048 {
		t.Fatalf("unexpected resource limits: %#v", body.HostConfig)
	}
	if len(body.HostConfig.DeviceRequests) != 1 || len(body.HostConfig.DeviceRequests[0].DeviceIDs) != 2 || body.HostConfig.DeviceRequests[0].DeviceIDs[1] != "GPU-B" {
		t.Fatalf("unexpected device request: %#v", body.HostConfig.DeviceRequests)
	}
}
