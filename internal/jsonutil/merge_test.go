package jsonutil

import (
	"encoding/json"
	"testing"
)

func TestMergeJSON(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		override string
		want     string
		wantErr  bool
	}{
		{
			name:     "empty override adds nothing",
			base:     `{"a":1}`,
			override: `{}`,
			want:     `{"a":1}`,
		},
		{
			name:     "override adds new key",
			base:     `{"a":1}`,
			override: `{"b":2}`,
			want:     `{"a":1,"b":2}`,
		},
		{
			name:     "override replaces scalar",
			base:     `{"a":1}`,
			override: `{"a":2}`,
			want:     `{"a":2}`,
		},
		{
			name:     "deep merge nested maps",
			base:     `{"gateway":{"providers":{"default":"ollama"},"http":{"port":8080}}}`,
			override: `{"gateway":{"providers":{"openai":{"apiKey":"sk-123"}}}}`,
			want:     `{"gateway":{"http":{"port":8080},"providers":{"default":"ollama","openai":{"apiKey":"sk-123"}}}}`,
		},
		{
			name:     "override replaces array",
			base:     `{"items":[1,2,3]}`,
			override: `{"items":[4,5]}`,
			want:     `{"items":[4,5]}`,
		},
		{
			name:     "override replaces map with scalar",
			base:     `{"a":{"nested":true}}`,
			override: `{"a":"replaced"}`,
			want:     `{"a":"replaced"}`,
		},
		{
			name:     "override replaces scalar with map",
			base:     `{"a":"string"}`,
			override: `{"a":{"nested":true}}`,
			want:     `{"a":{"nested":true}}`,
		},
		{
			name:     "invalid base JSON",
			base:     `not json`,
			override: `{}`,
			wantErr:  true,
		},
		{
			name:     "invalid override JSON",
			base:     `{}`,
			override: `not json`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergeJSON(tt.base, tt.override)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MergeJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			// Compare as parsed JSON to ignore key ordering
			var gotMap, wantMap map[string]any
			if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
				t.Fatalf("failed to parse got: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantMap); err != nil {
				t.Fatalf("failed to parse want: %v", err)
			}
			gotJSON, _ := json.Marshal(gotMap)
			wantJSON, _ := json.Marshal(wantMap)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("MergeJSON() = %s, want %s", got, tt.want)
			}
		})
	}
}
