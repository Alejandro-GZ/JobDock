//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jobdock/jobdock/internal/domain"
)

func discoverCPUPackages() []domain.CPUPackage {
	models := cpuModels("/proc/cpuinfo")
	paths, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*")
	samples := make([]cpuTopologySample, 0, len(paths))
	for _, path := range paths {
		logicalID, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(path), "cpu"))
		if err != nil {
			continue
		}
		packageData, packageErr := os.ReadFile(filepath.Join(path, "topology", "physical_package_id"))
		coreData, coreErr := os.ReadFile(filepath.Join(path, "topology", "core_id"))
		if packageErr != nil || coreErr != nil {
			continue
		}
		samples = append(samples, cpuTopologySample{logicalID: logicalID, packageID: strings.TrimSpace(string(packageData)), coreID: strings.TrimSpace(string(coreData)), model: models[logicalID]})
	}
	return buildCPUPackages(samples)
}

func cpuModels(path string) map[int]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[int]string{}
	}
	result := map[int]string{}
	for _, block := range strings.Split(string(data), "\n\n") {
		id := -1
		model := ""
		for _, line := range strings.Split(block, "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			switch strings.TrimSpace(parts[0]) {
			case "processor":
				id, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			case "model name":
				model = strings.TrimSpace(parts[1])
			}
		}
		if id >= 0 {
			result[id] = model
		}
	}
	return result
}
