package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
)

const maximumObservationMetadataBytes = 16 << 10

func validateObservationMetadata(metadata map[string]any) error {
	if metadata == nil {
		return nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("metadata must be valid JSON: %w", err)
	}
	if len(encoded) > maximumObservationMetadataBytes {
		return fmt.Errorf("metadata must not exceed 16 KiB")
	}
	keys := 0
	if err = validateObservationValue(metadata, 1, &keys); err != nil {
		return err
	}
	return nil
}

func validateObservationValue(value any, depth int, keys *int) error {
	if depth > 4 {
		return fmt.Errorf("metadata nesting must not exceed four levels")
	}
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			*keys++
			if *keys > 64 {
				return fmt.Errorf("metadata must contain at most 64 keys")
			}
			if key == "" || len(key) > 128 {
				return fmt.Errorf("metadata keys must contain 1-128 characters")
			}
			if err := validateObservationValue(child, depth+1, keys); err != nil {
				return err
			}
		}
	case []any:
		if len(item) > 64 {
			return fmt.Errorf("metadata arrays must contain at most 64 items")
		}
		for _, child := range item {
			if err := validateObservationValue(child, depth+1, keys); err != nil {
				return err
			}
		}
	case string:
		if len(item) > 1024 {
			return fmt.Errorf("metadata strings must contain at most 1024 characters")
		}
	case float64:
		if math.IsNaN(item) || math.IsInf(item, 0) {
			return fmt.Errorf("metadata numbers must be finite")
		}
	case nil, bool:
		return nil
	default:
		return fmt.Errorf("metadata contains an unsupported JSON value")
	}
	return nil
}
