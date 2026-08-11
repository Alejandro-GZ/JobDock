package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var lowercaseSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var ociDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func ValidateBuild(build Build) error {
	if n := len(strings.TrimSpace(build.Name)); n < 3 || n > 120 {
		return errors.New("build name must contain between 3 and 120 characters")
	}
	if build.Mode != BuildModeRailpack && build.Mode != BuildModeDockerfile {
		return errors.New("build mode must be RAILPACK or DOCKERFILE")
	}
	if strings.TrimSpace(build.Source.Filename) == "" || len(build.Source.Filename) > 255 {
		return errors.New("source filename is required and must not exceed 255 characters")
	}
	if build.Source.Size <= 0 || !lowercaseSHA256.MatchString(build.Source.SHA256) {
		return errors.New("source requires a positive size and lowercase SHA-256 digest")
	}
	if build.Status == BuildSucceeded && !ociDigest.MatchString(build.OCIDigest) {
		return errors.New("successful builds require an immutable sha256 OCI digest")
	}
	if build.Status != BuildSucceeded && build.OCIDigest != "" {
		return errors.New("only successful builds may reference an OCI digest")
	}
	if build.OCIDigest != "" && !ociDigest.MatchString(build.OCIDigest) {
		return errors.New("OCI digest must use sha256 with 64 lowercase hexadecimal characters")
	}
	if build.Status == BuildFailed && strings.TrimSpace(build.FailureReason) == "" {
		return errors.New("failed builds require a failure reason")
	}
	return nil
}

func ValidateBuildPlan(plan BuildPlan) error {
	if strings.TrimSpace(plan.BuildID) == "" {
		return errors.New("build plan requires a build ID")
	}
	if strings.TrimSpace(plan.Provider) == "" || len(plan.Provider) > 128 {
		return errors.New("build plan requires a detected provider")
	}
	if len(plan.Runtime) > 256 || len(plan.PackageManager) > 256 || len(plan.Entrypoint) > 8192 || len(plan.RailpackVersion) > 128 {
		return errors.New("build plan summary exceeds its allowed size")
	}
	if !json.Valid(plan.Plan) || !json.Valid(plan.Info) {
		return errors.New("build plan and build info must be valid JSON")
	}
	return nil
}

func ValidateJobSpec(spec JobSpec) error {
	if n := len(strings.TrimSpace(spec.Name)); n < 3 || n > 120 {
		return errors.New("name must contain between 3 and 120 characters")
	}
	if strings.TrimSpace(spec.Image) == "" || len(spec.Image) > 512 {
		return errors.New("image is required and must not exceed 512 characters")
	}
	if len(spec.Command) == 0 || len(spec.Command) > 128 {
		return errors.New("command must contain between 1 and 128 arguments")
	}
	for _, arg := range spec.Command {
		if len(arg) > 8192 {
			return errors.New("command arguments must not exceed 8192 characters")
		}
	}
	if spec.Resources.CPUMillis <= 0 || spec.Resources.MemoryBytes <= 0 {
		return errors.New("positive CPU and memory requests are required")
	}
	if spec.Resources.GPU.Count < 0 || spec.Resources.GPU.MinVRAMBytes < 0 {
		return errors.New("GPU requirements cannot be negative")
	}
	for key := range spec.Environment {
		if !envName.MatchString(key) || strings.HasPrefix(key, "JOBDOCK_") {
			return fmt.Errorf("invalid or reserved environment variable %q", key)
		}
	}
	for _, ref := range spec.SecretRefs {
		if ref.Name == "" || !envName.MatchString(ref.Target) {
			return errors.New("secret references require a name and valid target")
		}
		if ref.Mode != "file" && ref.Mode != "env" {
			return errors.New("secret mode must be file or env")
		}
	}
	if len(spec.Inputs) > 1024 {
		return errors.New("jobs may contain at most 1024 input files")
	}
	inputPaths := map[string]bool{}
	for _, input := range spec.Inputs {
		normalized := strings.ReplaceAll(input.Path, "\\", "/")
		if normalized == "" || path.IsAbs(normalized) || path.Clean(normalized) != normalized || normalized == "." || strings.HasPrefix(normalized, "../") {
			return fmt.Errorf("invalid input path %q", input.Path)
		}
		if inputPaths[normalized] {
			return fmt.Errorf("duplicate input path %q", input.Path)
		}
		inputPaths[normalized] = true
		if input.Size < 0 || len(input.SHA256) != 64 {
			return fmt.Errorf("input %q requires a non-negative size and SHA-256 digest", input.Path)
		}
		for _, character := range input.SHA256 {
			if !strings.ContainsRune("0123456789abcdef", character) {
				return fmt.Errorf("input %q has an invalid SHA-256 digest", input.Path)
			}
		}
	}
	return nil
}
