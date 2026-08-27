package agent

import (
	"sort"

	"github.com/jobdock/jobdock/internal/domain"
)

type cpuTopologySample struct {
	logicalID int
	packageID string
	coreID    string
	model     string
}

func buildCPUPackages(samples []cpuTopologySample) []domain.CPUPackage {
	packages := map[string]*domain.CPUPackage{}
	cores := map[string]map[string]bool{}
	for _, sample := range samples {
		if sample.logicalID < 0 || sample.packageID == "" {
			continue
		}
		item := packages[sample.packageID]
		if item == nil {
			item = &domain.CPUPackage{ID: sample.packageID, Model: sample.model}
			packages[sample.packageID] = item
			cores[sample.packageID] = map[string]bool{}
		}
		if item.Model == "" && sample.model != "" {
			item.Model = sample.model
		}
		item.LogicalCPUs = append(item.LogicalCPUs, sample.logicalID)
		cores[sample.packageID][sample.coreID] = true
	}
	result := make([]domain.CPUPackage, 0, len(packages))
	for id, item := range packages {
		sort.Ints(item.LogicalCPUs)
		item.PhysicalCores = len(cores[id])
		if item.PhysicalCores == 0 {
			item.PhysicalCores = len(item.LogicalCPUs)
		}
		item.TotalMillis = int64(len(item.LogicalCPUs)) * 1000
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
