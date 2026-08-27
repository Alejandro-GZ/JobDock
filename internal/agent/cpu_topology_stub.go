//go:build !linux

package agent

import "github.com/jobdock/jobdock/internal/domain"

func discoverCPUPackages() []domain.CPUPackage { return []domain.CPUPackage{} }
