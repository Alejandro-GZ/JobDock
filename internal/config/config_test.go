package config

import "testing"

func TestTelemetryRetentionCannotBeShorterThanRawWindow(t *testing.T) {
	t.Setenv("JOBDOCK_PUBLIC_URL", "https://dock.example.test")
	t.Setenv("JOBDOCK_TELEMETRY_RAW_RETENTION", "48h")
	t.Setenv("JOBDOCK_TELEMETRY_RETENTION", "24h")
	if _, err := LoadServer(); err == nil {
		t.Fatal("expected invalid telemetry retention to fail")
	}
}
