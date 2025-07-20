package main

import (
	"testing"
)

func TestParseAgentString(t *testing.T) {
	// Save the original builtinAgents and restore it after the test
	originalBuiltins := builtinAgents
	defer func() { builtinAgents = originalBuiltins }()

	// Set up some test builtin agents
	builtinAgents = map[string]string{
		"coder": "# test content",
		"auto":  "# test content",
	}

	tests := []struct {
		name           string
		input          string
		expectName     string
		expectPathFunc func(path string) bool // Function to verify path
	}{
		{
			name:       "Plus prefix builtin",
			input:      "+coder",
			expectName: "coder",
			expectPathFunc: func(path string) bool {
				return path == "builtin:coder"
			},
		},
		{
			name:       "Plus prefix user agent",
			input:      "+custom",
			expectName: "custom",
			expectPathFunc: func(path string) bool {
				return path != "" && path != "builtin:custom"
			},
		},
		{
			name:       "Absolute path",
			input:      "/path/to/agent.toml",
			expectName: "",
			expectPathFunc: func(path string) bool {
				return path == "/path/to/agent.toml"
			},
		},
		{
			name:       "Relative path with tilde",
			input:      "~/agents/custom.toml",
			expectName: "",
			expectPathFunc: func(path string) bool {
				return path != "~/agents/custom.toml" // Should be expanded
			},
		},
		{
			name:       "Name only for builtin",
			input:      "auto",
			expectName: "auto",
			expectPathFunc: func(path string) bool {
				return path == "builtin:auto"
			},
		},
		{
			name:       "Name only for user agent",
			input:      "custom",
			expectName: "custom",
			expectPathFunc: func(path string) bool {
				return path != "" && path != "builtin:custom"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, path := ParseAgentString(tt.input)

			if name != tt.expectName {
				t.Errorf("Expected name %q, got %q", tt.expectName, name)
			}

			if !tt.expectPathFunc(path) {
				t.Errorf("Path validation failed for %q, got %q", tt.input, path)
			}
		})
	}
}
