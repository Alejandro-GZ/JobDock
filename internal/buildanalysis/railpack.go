package buildanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxRailpackOutputBytes = 4 << 20

type Result struct {
	Provider        string
	Runtime         string
	PackageManager  string
	Entrypoint      string
	RailpackVersion string
	Plan            json.RawMessage
	Info            json.RawMessage
	Logs            []byte
}

type Analyzer interface {
	Analyze(context.Context, string) (Result, error)
}

type AnalysisError struct {
	Message string
	Logs    []byte
}

func (e *AnalysisError) Error() string { return e.Message }

type Railpack struct {
	Binary  string
	Timeout time.Duration
	Home    string
}

func (r *Railpack) WithHome(home string) *Railpack {
	r.Home = home
	return r
}

func NewRailpack(binary string, timeout time.Duration) *Railpack {
	if strings.TrimSpace(binary) == "" {
		binary = "railpack"
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &Railpack{Binary: binary, Timeout: timeout}
}

func (r *Railpack) Analyze(ctx context.Context, projectDir string) (Result, error) {
	binary, err := exec.LookPath(r.Binary)
	if err != nil {
		return Result{}, &AnalysisError{Message: "Railpack is unavailable. Configure JOBDOCK_RAILPACK_BINARY or use the official JobDock server image."}
	}
	home := r.Home
	if home == "" {
		home = os.TempDir()
	}
	if err = os.MkdirAll(home, 0o750); err != nil {
		return Result{}, err
	}
	temporaryRoot := filepath.Join(home, "tmp")
	if err = os.MkdirAll(temporaryRoot, 0o750); err != nil {
		return Result{}, err
	}
	outputDir, err := os.MkdirTemp(temporaryRoot, "jobdock-railpack-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(outputDir)
	planPath := filepath.Join(outputDir, "railpack-plan.json")
	infoPath := filepath.Join(outputDir, "railpack-info.json")
	commandCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, binary, "prepare", projectDir, "--plan-out", planPath, "--info-out", infoPath, "--error-missing-start")
	logs := &boundedBuffer{limit: maxRailpackOutputBytes}
	cmd.Stdout, cmd.Stderr = logs, logs
	cmd.Env = railpackEnvironment(home, temporaryRoot)
	runErr := cmd.Run()
	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		return Result{}, &AnalysisError{Message: fmt.Sprintf("Railpack analysis exceeded the %s time limit.", r.Timeout), Logs: logs.Bytes()}
	}
	if runErr != nil {
		message := actionableRailpackError(logs.String())
		return Result{}, &AnalysisError{Message: message, Logs: logs.Bytes()}
	}
	planJSON, err := readBoundedJSON(planPath)
	if err != nil {
		return Result{}, &AnalysisError{Message: "Railpack did not produce a valid build plan: " + err.Error(), Logs: logs.Bytes()}
	}
	infoJSON, err := readBoundedJSON(infoPath)
	if err != nil {
		return Result{}, &AnalysisError{Message: "Railpack did not produce valid build information: " + err.Error(), Logs: logs.Bytes()}
	}
	result, err := summarize(planJSON, infoJSON)
	if err != nil {
		return Result{}, &AnalysisError{Message: err.Error(), Logs: logs.Bytes()}
	}
	result.Plan, result.Info, result.Logs = planJSON, infoJSON, logs.Bytes()
	return result, nil
}

func summarize(planJSON, infoJSON json.RawMessage) (Result, error) {
	var info struct {
		RailpackVersion   string            `json:"railpackVersion"`
		Metadata          map[string]string `json:"metadata"`
		DetectedProviders []string          `json:"detectedProviders"`
		ResolvedPackages  map[string]struct {
			ResolvedVersion *string `json:"resolvedVersion"`
		} `json:"resolvedPackages"`
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(infoJSON, &info); err != nil {
		return Result{}, fmt.Errorf("invalid Railpack build information: %w", err)
	}
	if !info.Success || len(info.DetectedProviders) == 0 || strings.TrimSpace(info.DetectedProviders[0]) == "" {
		return Result{}, errors.New("Railpack could not detect a supported project. Add a supported project manifest or a railpack.json configuration with a provider and start command.")
	}
	var plan struct {
		Deploy struct {
			StartCommand string `json:"startCommand"`
		} `json:"deploy"`
	}
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return Result{}, fmt.Errorf("invalid Railpack build plan: %w", err)
	}
	provider := info.DetectedProviders[0]
	runtime := info.Metadata[provider+"Runtime"]
	if runtime == "" {
		runtime = firstMetadataSuffix(info.Metadata, "Runtime")
	}
	if runtime == "" {
		runtime = provider
	}
	packageManager := info.Metadata[provider+"PackageManager"]
	if packageManager == "" {
		packageManager = firstMetadataSuffix(info.Metadata, "PackageManager")
	}
	return Result{
		Provider:        provider,
		Runtime:         withResolvedVersion(runtime, info.ResolvedPackages),
		PackageManager:  withResolvedVersion(packageManager, info.ResolvedPackages),
		Entrypoint:      plan.Deploy.StartCommand,
		RailpackVersion: info.RailpackVersion,
	}, nil
}

func firstMetadataSuffix(metadata map[string]string, suffix string) string {
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := metadata[key]
		if strings.HasSuffix(key, suffix) && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func withResolvedVersion(name string, packages map[string]struct {
	ResolvedVersion *string `json:"resolvedVersion"`
}) string {
	if name == "" {
		return ""
	}
	if resolved, ok := packages[name]; ok && resolved.ResolvedVersion != nil && *resolved.ResolvedVersion != "" {
		return name + " " + *resolved.ResolvedVersion
	}
	return name
}

func actionableRailpackError(output string) string {
	clean := strings.TrimSpace(output)
	if clean == "" {
		return "Railpack could not analyze this project. Verify that the archive contains a supported project manifest and start command."
	}
	if len(clean) > 2000 {
		clean = clean[len(clean)-2000:]
	}
	return "Railpack could not analyze this project: " + clean + "\nAdd a supported project manifest or a railpack.json configuration with a provider and start command."
}

func readBoundedJSON(path string) (json.RawMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRailpackOutputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRailpackOutputBytes {
		return nil, errors.New("output exceeds 4 MiB")
	}
	if !json.Valid(data) {
		return nil, errors.New("output is not valid JSON")
	}
	return json.RawMessage(data), nil
}

func railpackEnvironment(home, temporaryRoot string) []string {
	environment := []string{"HOME=" + home, "TMPDIR=" + temporaryRoot, "TEMP=" + temporaryRoot, "TMP=" + temporaryRoot, "NO_COLOR=1", "FORCE_COLOR=0"}
	for _, key := range []string{"PATH", "SYSTEMROOT", "TEMP", "TMP"} {
		if key == "TEMP" || key == "TMP" {
			continue
		}
		if value := os.Getenv(key); value != "" {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
	cut    bool
}

func (w *boundedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data, w.cut = data[:remaining], true
		}
		_, _ = w.buffer.Write(data)
	} else {
		w.cut = true
	}
	return original, nil
}

func (w *boundedBuffer) Bytes() []byte {
	result := append([]byte(nil), w.buffer.Bytes()...)
	if w.cut {
		result = append(result, []byte("\n[JobDock truncated Railpack output]\n")...)
	}
	return result
}

func (w *boundedBuffer) String() string { return string(w.Bytes()) }
