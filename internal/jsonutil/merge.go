package jsonutil

import (
	"encoding/json"
	"fmt"
)

// MergeJSON deep-merges override JSON on top of base JSON.
// Scalars in override replace base values, maps merge recursively, arrays replace.
// Returns compacted JSON. Errors if either input is not valid JSON.
func MergeJSON(base, override string) (string, error) {
	var baseMap, overrideMap map[string]any
	if err := json.Unmarshal([]byte(base), &baseMap); err != nil {
		return "", fmt.Errorf("invalid base JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(override), &overrideMap); err != nil {
		return "", fmt.Errorf("invalid override JSON: %w", err)
	}
	deepMerge(baseMap, overrideMap)
	out, err := json.Marshal(baseMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged JSON: %w", err)
	}
	return string(out), nil
}

func deepMerge(base, override map[string]any) {
	for k, overrideVal := range override {
		baseVal, exists := base[k]
		if !exists {
			base[k] = overrideVal
			continue
		}
		baseMap, baseIsMap := baseVal.(map[string]any)
		overrideMap, overrideIsMap := overrideVal.(map[string]any)
		if baseIsMap && overrideIsMap {
			deepMerge(baseMap, overrideMap)
		} else {
			base[k] = overrideVal
		}
	}
}
