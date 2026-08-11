package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	ListenAddr             string
	DataDir                string
	DatabasePath           string
	PublicURL              string
	AllowInsecureHTTP      bool
	BootstrapUsername      string
	BootstrapPassword      string
	MasterKey              []byte
	SessionTTL             time.Duration
	HeartbeatOfflineAfter  time.Duration
	JobLostAfter           time.Duration
	MaxLogBytes            int64
	MaxOutputBytes         int64
	MaxInputBytes          int64
	TelemetryRawRetention  time.Duration
	TelemetryRetention     time.Duration
	BuildAnalysisTimeout   time.Duration
	RailpackBinary         string
	BuilderToken           string
	BuilderLease           time.Duration
	MaxBuildArtifactBytes  int64
	BuildArtifactRetention time.Duration
}

type Agent struct {
	ServerURL         string
	Token             string
	EnrollmentToken   string
	Name              string
	StateDir          string
	WorkspaceDir      string
	DockerSocket      string
	Labels            map[string]string
	AllowInsecureHTTP bool
	GPUMode           string
}

type Builder struct {
	ServerURL         string
	Token             string
	StateDir          string
	WorkspaceDir      string
	BuildctlBinary    string
	BuildkitAddress   string
	PollInterval      time.Duration
	Lease             time.Duration
	BuildTimeout      time.Duration
	MaxSourceBytes    int64
	MaxArtifactBytes  int64
	RailpackFrontend  string
	AllowInsecureHTTP bool
}

func LoadServer() (Server, error) {
	dataDir := env("JOBDOCK_DATA_DIR", ".jobdock/server")
	c := Server{
		ListenAddr:             env("JOBDOCK_LISTEN_ADDR", ":8080"),
		DataDir:                dataDir,
		DatabasePath:           env("JOBDOCK_DATABASE_PATH", filepath.Join(dataDir, "jobdock.db")),
		PublicURL:              strings.TrimRight(env("JOBDOCK_PUBLIC_URL", "http://localhost:8080"), "/"),
		AllowInsecureHTTP:      envBool("JOBDOCK_ALLOW_INSECURE_HTTP", false),
		BootstrapUsername:      env("JOBDOCK_BOOTSTRAP_ADMIN_USERNAME", "admin"),
		SessionTTL:             envDuration("JOBDOCK_SESSION_TTL", 24*time.Hour),
		HeartbeatOfflineAfter:  envDuration("JOBDOCK_HEARTBEAT_OFFLINE_AFTER", 30*time.Second),
		JobLostAfter:           envDuration("JOBDOCK_JOB_LOST_AFTER", 5*time.Minute),
		MaxLogBytes:            envInt64("JOBDOCK_MAX_LOG_BYTES", 10<<30),
		MaxOutputBytes:         envInt64("JOBDOCK_MAX_OUTPUT_BYTES", 100<<30),
		MaxInputBytes:          envInt64("JOBDOCK_MAX_INPUT_BYTES", 10<<30),
		TelemetryRawRetention:  envDuration("JOBDOCK_TELEMETRY_RAW_RETENTION", 24*time.Hour),
		TelemetryRetention:     envDuration("JOBDOCK_TELEMETRY_RETENTION", 30*24*time.Hour),
		BuildAnalysisTimeout:   envDuration("JOBDOCK_BUILD_ANALYSIS_TIMEOUT", 2*time.Minute),
		RailpackBinary:         env("JOBDOCK_RAILPACK_BINARY", "railpack"),
		BuilderLease:           envDuration("JOBDOCK_BUILDER_LEASE", 30*time.Second),
		MaxBuildArtifactBytes:  envInt64("JOBDOCK_MAX_BUILD_ARTIFACT_BYTES", 20<<30),
		BuildArtifactRetention: envDuration("JOBDOCK_BUILD_ARTIFACT_RETENTION", 30*24*time.Hour),
	}
	if c.MaxLogBytes <= 0 || c.MaxOutputBytes <= 0 || c.MaxInputBytes <= 0 {
		return c, errors.New("log, output, and input limits must be positive")
	}
	if c.TelemetryRawRetention <= 0 || c.TelemetryRetention < c.TelemetryRawRetention {
		return c, errors.New("telemetry retention must be positive and at least as long as raw retention")
	}
	if c.BuildAnalysisTimeout <= 0 || c.BuilderLease <= 0 || c.MaxBuildArtifactBytes <= 0 || c.BuildArtifactRetention <= 0 {
		return c, errors.New("build analysis timeout, builder lease, and build artifact limit must be positive")
	}
	password, err := valueOrFile("JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD", "JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD_FILE")
	if err != nil {
		return c, err
	}
	c.BootstrapPassword = password
	c.BuilderToken, err = valueOrFile("JOBDOCK_BUILDER_TOKEN", "JOBDOCK_BUILDER_TOKEN_FILE")
	if err != nil {
		return c, err
	}
	if c.BuilderToken != "" && len(c.BuilderToken) < 32 {
		return c, errors.New("JOBDOCK_BUILDER_TOKEN must contain at least 32 characters")
	}
	keyText, err := valueOrFile("JOBDOCK_MASTER_KEY", "JOBDOCK_MASTER_KEY_FILE")
	if err != nil {
		return c, err
	}
	if keyText != "" {
		key, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(keyText))
		if decodeErr != nil || len(key) != 32 {
			return c, errors.New("JOBDOCK_MASTER_KEY must be base64 for exactly 32 bytes")
		}
		c.MasterKey = key
	}
	if !c.AllowInsecureHTTP && strings.HasPrefix(c.PublicURL, "http://") {
		return c, errors.New("plain HTTP requires JOBDOCK_ALLOW_INSECURE_HTTP=true")
	}
	return c, nil
}

func LoadBuilder() (Builder, error) {
	token, err := valueOrFile("JOBDOCK_BUILDER_TOKEN", "JOBDOCK_BUILDER_TOKEN_FILE")
	if err != nil {
		return Builder{}, err
	}
	c := Builder{
		ServerURL:         strings.TrimRight(env("JOBDOCK_SERVER_URL", "http://jobdock-server:8080"), "/"),
		Token:             token,
		StateDir:          env("JOBDOCK_BUILDER_STATE_DIR", "/var/lib/jobdock-builder"),
		WorkspaceDir:      env("JOBDOCK_BUILDER_WORKSPACE_DIR", "/var/lib/jobdock-builder/workspaces"),
		BuildctlBinary:    env("JOBDOCK_BUILDCTL_BINARY", "buildctl"),
		BuildkitAddress:   env("JOBDOCK_BUILDKIT_ADDRESS", "tcp://buildkitd:1234"),
		PollInterval:      envDuration("JOBDOCK_BUILDER_POLL_INTERVAL", 2*time.Second),
		Lease:             envDuration("JOBDOCK_BUILDER_LEASE", 30*time.Second),
		BuildTimeout:      envDuration("JOBDOCK_BUILD_TIMEOUT", 30*time.Minute),
		MaxSourceBytes:    envInt64("JOBDOCK_MAX_INPUT_BYTES", 10<<30),
		MaxArtifactBytes:  envInt64("JOBDOCK_MAX_BUILD_ARTIFACT_BYTES", 20<<30),
		RailpackFrontend:  env("JOBDOCK_RAILPACK_FRONTEND", "ghcr.io/railwayapp/railpack-frontend:v0.36.0"),
		AllowInsecureHTTP: envBool("JOBDOCK_ALLOW_INSECURE_HTTP", false),
	}
	if len(c.Token) < 32 {
		return c, errors.New("JOBDOCK_BUILDER_TOKEN must contain at least 32 characters")
	}
	if c.PollInterval <= 0 || c.Lease <= c.PollInterval || c.BuildTimeout <= 0 || c.MaxSourceBytes <= 0 || c.MaxArtifactBytes <= 0 {
		return c, errors.New("builder intervals and storage limits are invalid")
	}
	if strings.HasPrefix(c.ServerURL, "http://") && !c.AllowInsecureHTTP {
		return c, errors.New("builder refuses plain HTTP unless JOBDOCK_ALLOW_INSECURE_HTTP=true")
	}
	return c, nil
}

func LoadAgent() (Agent, error) {
	c := Agent{
		ServerURL:         strings.TrimRight(env("JOBDOCK_SERVER_URL", "http://jobdock-server:8080"), "/"),
		Token:             env("JOBDOCK_AGENT_TOKEN", ""),
		EnrollmentToken:   env("JOBDOCK_ENROLLMENT_TOKEN", ""),
		Name:              env("JOBDOCK_NODE_NAME", hostname()),
		StateDir:          env("JOBDOCK_AGENT_STATE_DIR", "/var/lib/jobdock-agent"),
		WorkspaceDir:      env("JOBDOCK_WORKSPACE_DIR", "/var/lib/jobdock-agent/jobs"),
		DockerSocket:      env("JOBDOCK_DOCKER_SOCKET", "/var/run/docker.sock"),
		Labels:            parseLabels(env("JOBDOCK_NODE_LABELS", "")),
		AllowInsecureHTTP: envBool("JOBDOCK_ALLOW_INSECURE_HTTP", false),
		GPUMode:           strings.ToLower(env("JOBDOCK_GPU_MODE", "auto")),
	}
	if c.GPUMode != "auto" && c.GPUMode != "required" && c.GPUMode != "disabled" {
		return c, errors.New("JOBDOCK_GPU_MODE must be auto, required, or disabled")
	}
	if strings.HasPrefix(c.ServerURL, "http://") && !c.AllowInsecureHTTP {
		return c, errors.New("agent refuses plain HTTP unless JOBDOCK_ALLOW_INSECURE_HTTP=true")
	}
	return c, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func valueOrFile(valueKey, fileKey string) (string, error) {
	if value := os.Getenv(valueKey); value != "" {
		return value, nil
	}
	if filename := os.Getenv(fileKey); filename != "" {
		contents, err := os.ReadFile(filename)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", fileKey, err)
		}
		return strings.TrimSpace(string(contents)), nil
	}
	return "", nil
}

func parseLabels(value string) map[string]string {
	labels := map[string]string{}
	for _, item := range strings.Split(value, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) == 2 && parts[0] != "" {
			labels[parts[0]] = parts[1]
		}
	}
	return labels
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "jobdock-node"
	}
	return name
}
