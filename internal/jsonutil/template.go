package jsonutil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\{\{([A-Z][A-Z0-9_]*)\}\}`)

// ExtractPlaceholders returns sorted unique placeholder names from a template string.
// Placeholders use the syntax {{VARIABLE_NAME}} where names are uppercase with underscores.
func ExtractPlaceholders(template string) []string {
	matches := placeholderRe.FindAllStringSubmatch(template, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// SubstitutePlaceholders replaces all {{NAME}} placeholders with JSON-escaped values.
// Returns an error if any placeholder in the template has no corresponding value.
func SubstitutePlaceholders(template string, values map[string]string) (string, error) {
	placeholders := ExtractPlaceholders(template)
	var missing []string
	for _, name := range placeholders {
		if _, ok := values[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required placeholder values: %s", strings.Join(missing, ", "))
	}

	result := placeholderRe.ReplaceAllStringFunc(template, func(match string) string {
		name := placeholderRe.FindStringSubmatch(match)[1]
		// JSON-encode the value to handle special characters, then strip outer quotes
		// since the placeholder is typically already inside quotes in the template.
		encoded, _ := json.Marshal(values[name])
		// Strip the surrounding quotes from json.Marshal output
		return string(encoded[1 : len(encoded)-1])
	})

	return result, nil
}

// ValidateTemplate checks that a template with placeholders produces valid JSON
// when dummy values are substituted. Returns nil if there are no placeholders
// (caller should use json.Valid for plain JSON).
func ValidateTemplate(template string) error {
	placeholders := ExtractPlaceholders(template)
	if len(placeholders) == 0 {
		return nil
	}
	dummy := make(map[string]string, len(placeholders))
	for _, name := range placeholders {
		dummy[name] = "placeholder"
	}
	result, err := SubstitutePlaceholders(template, dummy)
	if err != nil {
		return fmt.Errorf("template substitution failed: %w", err)
	}
	if !json.Valid([]byte(result)) {
		return fmt.Errorf("template does not produce valid JSON after substitution")
	}
	return nil
}
