package main

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestValidateAgent(t *testing.T) {
	tests := []struct {
		name        string
		agentConfig string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid agent",
			agentConfig: `
name = "test-agent"
description = "A test agent"

[[functions]]
name = "hello"
description = "Say hello"
command = "echo Hello"
safe = true
`,
			wantErr: false,
		},
		{
			name: "function with missing name",
			agentConfig: `
name = "test-agent"
description = "A test agent"

[[functions]]
description = "Function without name"
command = "echo Hello"
`,
			wantErr:     true,
			errContains: "has no name",
		},
		{
			name: "function with missing command",
			agentConfig: `
name = "test-agent"
description = "A test agent"

[[functions]]
name = "hello"
description = "Function without command"
`,
			wantErr:     true,
			errContains: "has no command defined",
		},
		{
			name: "parameter with invalid type",
			agentConfig: `
name = "test-agent"
description = "A test agent"

[[functions]]
name = "hello"
description = "Function with invalid parameter type"
command = "echo Hello"

[[functions.parameters]]
name = "param1"
type = "invalid-type"
description = "A parameter with invalid type"
`,
			wantErr:     true,
			errContains: "invalid type",
		},
		{
			name: "invalid ask level",
			agentConfig: `
name = "test-agent"
ask = "invalid"
`,
			wantErr:     true,
			errContains: "invalid ask level",
		},
		{
			name: "valid ask level none",
			agentConfig: `
name = "test-agent"
ask = "none"
`,
			wantErr: false,
		},
		{
			name: "valid ask level all",
			agentConfig: `
name = "test-agent"
ask = "all"
`,
			wantErr: false,
		},
		{
			name: "duplicate function name",
			agentConfig: `
name = "test-agent"

[[functions]]
name = "hello"
description = "Say hello"
command = "echo Hello"

[[functions]]
name = "hello"
description = "Duplicate hello"
command = "echo World"
`,
			wantErr:     true,
			errContains: "duplicate function name",
		},
		{
			name: "duplicate parameter name",
			agentConfig: `
name = "test-agent"

[[functions]]
name = "hello"
description = "Say hello"
command = "echo {{a}} {{a}}"

[[functions.parameters]]
name = "a"
type = "string"
description = "first"

[[functions.parameters]]
name = "a"
type = "string"
description = "second"
`,
			wantErr:     true,
			errContains: "duplicate parameter name",
		},
		{
			name: "invalid timeout too high",
			agentConfig: `
name = "test-agent"

[[functions]]
name = "hello"
description = "Say hello"
command = "echo Hello"
timeout = 7200
`,
			wantErr:     true,
			errContains: "invalid timeout",
		},
		{
			name: "valid timeout",
			agentConfig: `
name = "test-agent"

[[functions]]
name = "hello"
description = "Say hello"
command = "echo Hello"
timeout = 300
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var agent Agent
			if _, err := toml.Decode(tt.agentConfig, &agent); err != nil {
				t.Fatalf("Failed to decode agent config: %v", err)
			}

			_, err := validateAgent(agent)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAgent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateAgent() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}
