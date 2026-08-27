package agent

import "testing"

func TestBuildCPUPackagesHandlesSocketsThreadsAndGaps(t *testing.T) {
	packages := buildCPUPackages([]cpuTopologySample{
		{logicalID: 0, packageID: "0", coreID: "0", model: "CPU A"},
		{logicalID: 4, packageID: "0", coreID: "0", model: "CPU A"},
		{logicalID: 2, packageID: "1", coreID: "7", model: "CPU B"},
		{logicalID: 6, packageID: "1", coreID: "8", model: "CPU B"},
	})
	if len(packages) != 2 || packages[0].PhysicalCores != 1 || packages[0].TotalMillis != 2000 || packages[0].LogicalCPUs[1] != 4 {
		t.Fatalf("unexpected topology: %#v", packages)
	}
	if packages[1].ID != "1" || packages[1].PhysicalCores != 2 || packages[1].Model != "CPU B" {
		t.Fatalf("unexpected second package: %#v", packages[1])
	}
}

func TestBuildCPUPackagesSkipsIncompleteSamples(t *testing.T) {
	packages := buildCPUPackages([]cpuTopologySample{{logicalID: -1, packageID: "0"}, {logicalID: 1}})
	if len(packages) != 0 {
		t.Fatalf("expected incomplete samples to be ignored: %#v", packages)
	}
}
