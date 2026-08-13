package httpapi

import (
	"strings"
	"testing"
)

func TestObservationMetadataValidationIsBounded(t *testing.T) {
	valid := map[string]any{"split": "train", "context": map[string]any{"fold": float64(2)}, "tags": []any{"ml", true}}
	if err := validateObservationMetadata(valid); err != nil {
		t.Fatalf("valid metadata: %v", err)
	}
	for name, metadata := range map[string]map[string]any{
		"long string": {"value": strings.Repeat("x", 1025)},
		"deep":        {"a": map[string]any{"b": map[string]any{"c": map[string]any{"d": "too deep"}}}},
		"wide array":  {"values": make([]any, 65)},
	} {
		if err := validateObservationMetadata(metadata); err == nil {
			t.Errorf("%s metadata was accepted", name)
		}
	}
}
