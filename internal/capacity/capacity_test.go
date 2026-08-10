package capacity

import (
	"testing"

	"github.com/jobdock/jobdock/internal/domain"
)

func TestAccountReservations(t *testing.T) {
	nodes := []domain.Node{{ID: "node-1", GPUs: []domain.GPU{{UUID: "GPU-1"}, {UUID: "GPU-2"}}}}
	jobs := []domain.Job{{AssignedNodeID: "node-1", Status: domain.JobRunning, Spec: domain.JobSpec{Resources: domain.Resources{CPUMillis: 1500, MemoryBytes: 2 << 30}}}}
	Account(nodes, jobs, map[string]map[string]bool{"node-1": {"GPU-2": true}})
	if nodes[0].CPUAllocatedMillis != 1500 || nodes[0].MemoryAllocatedBytes != 2<<30 {
		t.Fatalf("unexpected allocation: %#v", nodes[0])
	}
	if nodes[0].GPUs[0].Allocated || !nodes[0].GPUs[1].Allocated {
		t.Fatalf("unexpected GPU allocation: %#v", nodes[0].GPUs)
	}
}
