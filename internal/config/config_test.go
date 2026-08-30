package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemetryRetentionCannotBeShorterThanRawWindow(t *testing.T) {
	t.Setenv("JOBDOCK_PUBLIC_URL", "https://dock.example.test")
	t.Setenv("JOBDOCK_TELEMETRY_RAW_RETENTION", "48h")
	t.Setenv("JOBDOCK_TELEMETRY_RETENTION", "24h")
	if _, err := LoadServer(); err == nil {
		t.Fatal("expected invalid telemetry retention to fail")
	}
}

func TestBuilderRequiresScopedCredentialAndExplicitHTTPOptIn(t *testing.T) {
	t.Setenv("JOBDOCK_BUILDER_TOKEN", "builder-token-with-at-least-32-characters")
	t.Setenv("JOBDOCK_SERVER_URL", "http://server.test")
	if _, err := LoadBuilder(); err == nil {
		t.Fatal("plain HTTP builder connection was accepted without opt-in")
	}
	t.Setenv("JOBDOCK_ALLOW_INSECURE_HTTP", "true")
	if _, err := LoadBuilder(); err != nil {
		t.Fatal(err)
	}
}

func TestServerLoadsInternalCredentialsFromFiles(t *testing.T) {
	root := t.TempDir()
	setupPath := filepath.Join(root, "setup")
	builderPath := filepath.Join(root, "builder")
	masterPath := filepath.Join(root, "master")
	for path, value := range map[string]string{
		setupPath:   strings.Repeat("s", 48),
		builderPath: strings.Repeat("b", 48),
		masterPath:  base64.StdEncoding.EncodeToString(make([]byte, 32)),
	} {
		if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("JOBDOCK_PUBLIC_URL", "https://dock.example.test")
	t.Setenv("JOBDOCK_SETUP_TOKEN_FILE", setupPath)
	t.Setenv("JOBDOCK_BUILDER_TOKEN_FILE", builderPath)
	t.Setenv("JOBDOCK_MASTER_KEY_FILE", masterPath)
	cfg, err := LoadServer()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SetupToken != strings.Repeat("s", 48) || cfg.BuilderToken != strings.Repeat("b", 48) || len(cfg.MasterKey) != 32 {
		t.Fatalf("file-backed credentials were not loaded: %#v", cfg)
	}
}

func TestServerRejectsInvalidOverridesBeforeStartup(t *testing.T) {
	t.Setenv("JOBDOCK_PUBLIC_URL", "https://dock.example.test")
	t.Setenv("JOBDOCK_TELEMETRY_RETENTION", "not-a-duration")
	if _, err := LoadServer(); err == nil || !strings.Contains(err.Error(), "JOBDOCK_TELEMETRY_RETENTION") {
		t.Fatalf("invalid duration was not rejected: %v", err)
	}
	t.Setenv("JOBDOCK_TELEMETRY_RETENTION", "720h")
	t.Setenv("JOBDOCK_ALLOW_INSECURE_HTTP", "sometimes")
	if _, err := LoadServer(); err == nil || !strings.Contains(err.Error(), "JOBDOCK_ALLOW_INSECURE_HTTP") {
		t.Fatalf("invalid boolean was not rejected: %v", err)
	}
}

func TestServerRejectsShortSetupToken(t *testing.T) {
	t.Setenv("JOBDOCK_PUBLIC_URL", "https://dock.example.test")
	t.Setenv("JOBDOCK_SETUP_TOKEN", "short")
	if _, err := LoadServer(); err == nil || !strings.Contains(err.Error(), "JOBDOCK_SETUP_TOKEN") {
		t.Fatalf("short setup token was not rejected: %v", err)
	}
}
