package store

import (
	"strings"
	"testing"
)

func TestMetricDescriptorQueryIsIndependentOfSampleHistory(t *testing.T) {
	query := metricDescriptorQuery(2)
	if strings.Contains(query, "job_metric_samples") {
		t.Fatalf("catalog query must not read sample history: %s", query)
	}
	if strings.Count(query, "AND EXISTS (SELECT 1 FROM json_each") != 2 || !strings.Contains(query, "job_metric_descriptors") {
		t.Fatalf("catalog query must apply one descriptor-level predicate per tag: %s", query)
	}
}
