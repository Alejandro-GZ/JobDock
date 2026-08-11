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
	ListenAddr            string
	DataDir               string
	DatabasePath          string
	PublicURL             string
	AllowInsecureHTTP     bool
	BootstrapUsername     string
	BootstrapPassword     string
	MasterKey             []byte
	SessionTTL            time.Duration
	HeartbeatOfflineAfter time.Duration
	JobLostAfter          time.Duration
	MaxLogBytes           int64
	MaxOutputBytes        int64
	TelemetryRawRetention time.Duration
	TelemetryRetention    time.Duration
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

func LoadServer() (Server, error) {
	dataDir := env("JOBDOCK_DATA_DIR", ".jobdock/server")
	c := Server{
		ListenAddr:            env("JOBDOCK_LISTEN_ADDR", ":8080"),
		DataDir:               dataDir,
		DatabasePath:          env("JOBDOCK_DATABASE_PATH", filepath.Join(dataDir, "jobdock.db")),
		PublicURL:             strings.TrimRight(env("JOBDOCK_PUBLIC_URL", "http://localhost:8080"), "/"),
		AllowInsecureHTTP:     envBool("JOBDOCK_ALLOW_INSECURE_HTTP", false),
		BootstrapUsername:     env("JOBDOCK_BOOTSTRAP_ADMIN_USERNAME", "admin"),
		SessionTTL:            envDuration("JOBDOCK_SESSION_TTL", 24*time.Hour),
		HeartbeatOfflineAfter: envDuration("JOBDOCK_HEARTBEAT_OFFLINE_AFTER", 30*time.Second),
		JobLostAfter:          envDuration("JOBDOCK_JOB_LOST_AFTER", 5*time.Minute),
		MaxLogBytes:           envInt64("JOBDOCK_MAX_LOG_BYTES", 10<<30),
		MaxOutputBytes:        envInt64("JOBDOCK_MAX_OUTPUT_BYTES", 100<<30),
		TelemetryRawRetention: envDuration("JOBDOCK_TELEMETRY_RAW_RETENTION", 24*time.Hour),
		TelemetryRetention:    envDuration("JOBDOCK_TELEMETRY_RETENTION", 30*24*time.Hour),
	}
	if c.TelemetryRawRetention <= 0 || c.TelemetryRetention < c.TelemetryRawRetention {
		return c, errors.New("telemetry retention must be positive and at least as long as raw retention")
	}
	password, err := valueOrFile("JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD", "JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD_FILE")
	if err != nil {
		return c, err
	}
	c.BootstrapPassword = password
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
