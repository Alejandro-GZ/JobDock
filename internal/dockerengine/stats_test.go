package dockerengine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDockerStatsAreNormalizedWithoutRawPayload(t *testing.T) {
	const payload = `{
		"cpu_stats":{"cpu_usage":{"total_usage":3000000000,"percpu_usage":[1,1,1,1]},"system_cpu_usage":12000000000,"online_cpus":4},
		"precpu_stats":{"cpu_usage":{"total_usage":1000000000},"system_cpu_usage":8000000000},
		"memory_stats":{"usage":1073741824,"stats":{"inactive_file":268435456,"pgfault":999}},
		"networks":{"eth0":{"rx_bytes":999999999}},"blkio_stats":{"io_service_bytes_recursive":[{"value":42}]}
	}`
	var raw dockerStats
	if err := json.NewDecoder(strings.NewReader(payload)).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	stats := normalizeStats(raw)
	if stats.CPUMillis != 2000 || stats.MemoryBytes != 768<<20 {
		t.Fatalf("normalized stats: %#v", stats)
	}
}
