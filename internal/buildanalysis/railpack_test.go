package buildanalysis

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSummarizeUsesOnlyRailpackOutput(t *testing.T) {
	plan := json.RawMessage(`{"deploy":{"startCommand":"npm run start"}}`)
	info := json.RawMessage(`{"success":true,"railpackVersion":"0.36.0","detectedProviders":["node"],"metadata":{"nodeRuntime":"node","nodePackageManager":"pnpm"}}`)
	result, err := summarize(plan, info)
	if err != nil || result.Provider != "node" || result.Runtime != "node" || result.PackageManager != "pnpm" || result.Entrypoint != "npm run start" || result.RailpackVersion != "0.36.0" {
		t.Fatalf("summary=%#v error=%v", result, err)
	}
}

func TestRailpackPrepareIntegration(t *testing.T) {
	binary := os.Getenv("JOBDOCK_RAILPACK_INTEGRATION_BINARY")
	if binary == "" {
		t.Skip("set JOBDOCK_RAILPACK_INTEGRATION_BINARY to run the pinned CLI integration")
	}
	analyzer := NewRailpack(binary, time.Minute).WithHome(t.TempDir())
	result, err := analyzer.Analyze(context.Background(), "testdata/python")
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "python" || result.Runtime == "" || result.PackageManager != "pip" || result.Entrypoint == "" || result.RailpackVersion != "v0.36.0" {
		t.Fatalf("Railpack summary provider=%q runtime=%q package_manager=%q entrypoint=%q version=%q", result.Provider, result.Runtime, result.PackageManager, result.Entrypoint, result.RailpackVersion)
	}
	_, err = analyzer.Analyze(context.Background(), "testdata/unsupported")
	if err == nil || !strings.Contains(err.Error(), "supported project manifest") || !strings.Contains(err.Error(), "railpack.json") {
		t.Fatalf("unsupported project error=%v", err)
	}
}

func TestSummarizeRejectsUnsupportedProjectActionably(t *testing.T) {
	_, err := summarize(json.RawMessage(`{"deploy":{}}`), json.RawMessage(`{"success":false}`))
	if err == nil || !strings.Contains(err.Error(), "supported project manifest") || !strings.Contains(err.Error(), "railpack.json") {
		t.Fatalf("error = %v", err)
	}
}

func TestBoundedBufferDoesNotBlockRailpackAndMarksTruncation(t *testing.T) {
	buffer := &boundedBuffer{limit: 4}
	if written, err := buffer.Write([]byte("long output")); err != nil || written != 11 {
		t.Fatalf("write=%d error=%v", written, err)
	}
	if !strings.Contains(buffer.String(), "long") || !strings.Contains(buffer.String(), "truncated") {
		t.Fatalf("buffer=%q", buffer.String())
	}
}
