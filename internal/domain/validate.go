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
	if build.Mode == BuildModeDockerfile {
		contextPath, dockerfilePath := build.ContextPath, build.DockerfilePath
		if contextPath == "" {
			contextPath = "."
		}
		if dockerfilePath == "" {
			dockerfilePath = "Dockerfile"
		}
		if err := validateBuildRelativePath(contextPath, true); err != nil {
			return fmt.Errorf("invalid build context: %w", err)
		}
		if err := validateBuildRelativePath(dockerfilePath, false); err != nil {
			return fmt.Errorf("invalid Dockerfile path: %w", err)
		}
	} else if build.ContextPath != "" || build.DockerfilePath != "" {
		return errors.New("Railpack builds cannot define Dockerfile context settings")
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

func validateBuildRelativePath(value string, allowDot bool) error {
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "\\:\x00") || strings.HasPrefix(value, "/") {
		return errors.New("path must be a slash-separated relative path of at most 512 characters")
	}
	cleaned := path.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || (!allowDot && cleaned == ".") || cleaned != value {
		return errors.New("path must be normalized and remain inside the source archive")
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

func ValidateBuildAssignment(assignment BuildAssignment) error {
	if strings.TrimSpace(assignment.ID) == "" || strings.TrimSpace(assignment.BuildID) == "" {
		return errors.New("build assignment requires assignment and build IDs")
	}
	switch assignment.Status {
	case BuildAssignmentPending, BuildAssignmentRunning, BuildAssignmentSucceeded, BuildAssignmentFailed, BuildAssignmentCancelled:
	default:
		return errors.New("invalid build assignment status")
	}
	if assignment.Status == BuildAssignmentRunning && strings.TrimSpace(assignment.BuilderID) == "" {
		return errors.New("running build assignments require a builder ID")
	}
	return nil
}

func ValidateManagedArtifact(artifact ManagedArtifact) error {
	if strings.TrimSpace(artifact.BuildID) == "" || strings.TrimSpace(artifact.OwnerID) == "" {
		return errors.New("managed artifact requires build and owner IDs")
	}
	if !ociDigest.MatchString(artifact.Digest) || !lowercaseSHA256.MatchString(artifact.SHA256) {
		return errors.New("managed artifact requires immutable OCI and archive SHA-256 digests")
	}
	if artifact.Size <= 0 || artifact.MediaType != ManagedImageMediaType {
		return errors.New("managed artifact size or media type is invalid")
	}
	if artifact.RuntimeImage != "jobdock.local/managed/"+artifact.BuildID+":artifact" {
		return errors.New("managed artifact runtime image does not match its build")
	}
	if artifact.CreatedAt.IsZero() || artifact.LastReferencedAt.IsZero() {
		return errors.New("managed artifact timestamps are required")
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
	if len(spec.Command) > 128 {
		return errors.New("command must not contain more than 128 arguments")
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
	if len(spec.Resources.GPU.UUIDs) > 0 {
		if spec.TargetNodeID == "" {
			return errors.New("target_node_id is required for explicit GPU selection")
		}
		if spec.Resources.GPU.Count != len(spec.Resources.GPU.UUIDs) {
			return errors.New("GPU count must match the number of explicit UUIDs")
		}
		seen := map[string]bool{}
		for _, id := range spec.Resources.GPU.UUIDs {
			if strings.TrimSpace(id) == "" || seen[id] {
				return errors.New("explicit GPU UUIDs must be non-empty and unique")
			}
			seen[id] = true
		}
	}
	if spec.Resources.CPUPackageID != "" && spec.TargetNodeID == "" {
		return errors.New("target_node_id is required for CPU package affinity")
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
