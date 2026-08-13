package builder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
)

type Executor interface {
	Build(context.Context, domain.BuildWork, string, string, io.Writer) (string, error)
}

type BuildKit struct {
	config config.Builder
}

func NewBuildKit(cfg config.Builder) *BuildKit { return &BuildKit{config: cfg} }

func (b *BuildKit) Build(ctx context.Context, work domain.BuildWork, projectDir, artifactPath string, logs io.Writer) (string, error) {
	metadataPath := artifactPath + ".metadata.json"
	partialPath := artifactPath + ".partial"
	_ = os.Remove(metadataPath)
	_ = os.Remove(partialPath)
	defer os.Remove(metadataPath)
	defer os.Remove(partialPath)
	args := []string{"--addr", b.config.BuildkitAddress, "build", "--progress=plain"}
	var planDir string
	var err error
	switch work.Build.Mode {
	case domain.BuildModeRailpack:
		if work.Plan == nil || len(work.Plan.Plan) == 0 {
			return "", errors.New("confirmed Railpack build plan is missing")
		}
		planDir, err = os.MkdirTemp(filepath.Dir(artifactPath), ".railpack-plan-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(planDir)
		if err = os.WriteFile(filepath.Join(planDir, "railpack-plan.json"), work.Plan.Plan, 0o440); err != nil {
			return "", err
		}
		args = append(args, "--frontend=gateway.v0", "--opt", "source="+b.config.RailpackFrontend, "--local", "context="+projectDir, "--local", "dockerfile="+planDir)
	case domain.BuildModeDockerfile:
		contextPath, dockerfilePath := work.Build.ContextPath, work.Build.DockerfilePath
		if contextPath == "" {
			contextPath = "."
		}
		if dockerfilePath == "" {
			dockerfilePath = "Dockerfile"
		}
		contextDir, resolveErr := sourcePath(projectDir, contextPath)
		if resolveErr != nil {
			return "", fmt.Errorf("invalid build context: %w", resolveErr)
		}
		dockerfile, resolveErr := sourcePath(projectDir, dockerfilePath)
		if resolveErr != nil {
			return "", fmt.Errorf("invalid Dockerfile path: %w", resolveErr)
		}
		if info, statErr := os.Lstat(contextDir); statErr != nil || !info.IsDir() {
			return "", errors.New("configured build context is not a directory")
		}
		if info, statErr := os.Lstat(dockerfile); statErr != nil || !info.Mode().IsRegular() {
			return "", errors.New("configured Dockerfile is not a regular file")
		}
		args = append(args, "--frontend=dockerfile.v0", "--local", "context="+contextDir, "--local", "dockerfile="+filepath.Dir(dockerfile), "--opt", "filename="+filepath.Base(dockerfile))
	default:
		return "", fmt.Errorf("unsupported build mode %q", work.Build.Mode)
	}
	args = append(args, "--output", "type=docker,name="+managedRuntimeImage(work.Build.ID)+",dest="+partialPath, "--metadata-file", metadataPath)
	command := exec.CommandContext(ctx, b.config.BuildctlBinary, args...)
	command.Stdout, command.Stderr = logs, logs
	command.Env = minimalEnvironment()
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("BuildKit execution failed: %w", err)
	}
	info, err := os.Lstat(partialPath)
	if err != nil {
		return "", fmt.Errorf("BuildKit did not produce an OCI archive: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return "", errors.New("BuildKit produced an invalid OCI archive")
	}
	if info.Size() > b.config.MaxArtifactBytes {
		return "", fmt.Errorf("OCI artifact exceeds the configured %d-byte limit", b.config.MaxArtifactBytes)
	}
	digest, err := readBuildDigest(metadataPath)
	if err != nil {
		return "", err
	}
	if err = os.Rename(partialPath, artifactPath); err != nil {
		return "", err
	}
	return digest, nil
}

func sourcePath(root, relative string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(root, candidate)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the source archive")
	}
	return candidate, nil
}

func managedRuntimeImage(buildID string) string {
	return "jobdock.local/managed/" + buildID + ":artifact"
}

func readBuildDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read BuildKit result metadata: %w", err)
	}
	var metadata map[string]json.RawMessage
	if err = json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("decode BuildKit result metadata: %w", err)
	}
	var digest string
	_ = json.Unmarshal(metadata["containerimage.digest"], &digest)
	if digest == "" {
		var descriptor struct {
			Digest string `json:"digest"`
		}
		_ = json.Unmarshal(metadata["containerimage.descriptor"], &descriptor)
		digest = descriptor.Digest
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
		return "", errors.New("BuildKit result does not contain an immutable sha256 OCI digest")
	}
	return digest, nil
}

func minimalEnvironment() []string {
	allowed := []string{"HOME", "PATH", "SYSTEMROOT", "TEMP", "TMP"}
	environment := []string{"NO_COLOR=1", "BUILDKIT_PROGRESS=plain"}
	for _, key := range allowed {
		if value := os.Getenv(key); value != "" {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}
