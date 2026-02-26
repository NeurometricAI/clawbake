package jsonutil

import (
	"strings"
	"testing"
)

func TestExtractPlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     []string
	}{
		{
			name:     "no placeholders",
			template: `{"key": "value"}`,
			want:     nil,
		},
		{
			name:     "single placeholder",
			template: `{"apiKey": "{{API_KEY}}"}`,
			want:     []string{"API_KEY"},
		},
		{
			name:     "multiple placeholders sorted",
			template: `{"b": "{{ZZZ_KEY}}", "a": "{{AAA_KEY}}"}`,
			want:     []string{"AAA_KEY", "ZZZ_KEY"},
		},
		{
			name:     "duplicate placeholders deduplicated",
			template: `{"a": "{{KEY}}", "b": "{{KEY}}"}`,
			want:     []string{"KEY"},
		},
		{
			name:     "lowercase ignored",
			template: `{"a": "{{lowercase}}"}`,
			want:     nil,
		},
		{
			name:     "mixed case ignored",
			template: `{"a": "{{mixedCase}}"}`,
			want:     nil,
		},
		{
			name:     "numbers in name",
			template: `{"a": "{{API_KEY_2}}"}`,
			want:     []string{"API_KEY_2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPlaceholders(tt.template)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractPlaceholders() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExtractPlaceholders()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSubstitutePlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		template string
		values   map[string]string
		want     string
		wantErr  string
	}{
		{
			name:     "all values supplied",
			template: `{"apiKey": "{{API_KEY}}"}`,
			values:   map[string]string{"API_KEY": "sk-123"},
			want:     `{"apiKey": "sk-123"}`,
		},
		{
			name:     "missing value",
			template: `{"a": "{{KEY_A}}", "b": "{{KEY_B}}"}`,
			values:   map[string]string{"KEY_A": "val"},
			wantErr:  "KEY_B",
		},
		{
			name:     "special chars auto-escaped",
			template: `{"val": "{{VALUE}}"}`,
			values:   map[string]string{"VALUE": `he said "hello" & \n`},
			want:     `{"val": "he said \"hello\" \u0026 \\n"}`,
		},
		{
			name:     "multiple occurrences replaced",
			template: `{"a": "{{KEY}}", "b": "{{KEY}}"}`,
			values:   map[string]string{"KEY": "val"},
			want:     `{"a": "val", "b": "val"}`,
		},
		{
			name:     "no placeholders",
			template: `{"key": "value"}`,
			values:   map[string]string{},
			want:     `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SubstitutePlaceholders(tt.template, tt.values)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("SubstitutePlaceholders() expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("SubstitutePlaceholders() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SubstitutePlaceholders() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("SubstitutePlaceholders() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{
			name:     "valid template",
			template: `{"apiKey": "{{API_KEY}}", "port": 8080}`,
			wantErr:  false,
		},
		{
			name:     "malformed structure",
			template: `{"apiKey": {{API_KEY}}}`,
			wantErr:  true,
		},
		{
			name:     "no placeholders returns nil",
			template: `{"key": "value"}`,
			wantErr:  false,
		},
		{
			name:     "multiple placeholders valid",
			template: `{"a": "{{KEY_A}}", "b": "{{KEY_B}}"}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplate(tt.template)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
