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
			name: "missing agent name",
			agentConfig: `
description = "A test agent without name"

[[functions]]
name = "hello"
description = "Say hello"
command = "echo Hello"
`,
			wantErr:     true,
			errContains: "agent has no name",
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
			name: "mcp server without command",
			agentConfig: `
name = "test-agent"
description = "A test agent"

[mcp_servers.server1]
args = ["--port", "8080"]
`,
			wantErr:     true,
			errContains: "has no command defined",
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
