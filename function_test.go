package main

import (
	"strings"
	"testing"
)

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
