package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetConversationIndex(t *testing.T) {
	tests := []struct {
		name         string
		conversation string
		wantIndex    int
		wantIsIndex  bool
	}{
		{
			name:         "Valid positive number",
			conversation: "1",
			wantIndex:    0,
			wantIsIndex:  true,
		},
		{
			name:         "Valid larger number",
			conversation: "5",
			wantIndex:    4,
			wantIsIndex:  true,
		},
		{
			name:         "Zero is valid",
			conversation: "0",
			wantIndex:    -1,
			wantIsIndex:  true,
		},
		{
			name:         "Custom conversation ID",
			conversation: "my-custom-id",
			wantIndex:    0,
			wantIsIndex:  false,
		},
		{
			name:         "Mixed alphanumeric",
			conversation: "session123",
			wantIndex:    0,
			wantIsIndex:  false,
		},
		{
			name:         "Empty string",
			conversation: "",
			wantIndex:    0,
			wantIsIndex:  false,
		},
		{
			name:         "Negative number",
			conversation: "-1",
			wantIndex:    0,
			wantIsIndex:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex, gotIsIndex := getConversationIndex(tt.conversation)
			if gotIndex != tt.wantIndex {
				t.Errorf("getConversationIndex() index = %v, want %v", gotIndex, tt.wantIndex)
			}
			if gotIsIndex != tt.wantIsIndex {
				t.Errorf("getConversationIndex() isIndex = %v, want %v", gotIsIndex, tt.wantIsIndex)
			}
		})
	}
}

func TestCreateNewHistoryFile(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name         string
		agentName    string
		conversation string
		wantPattern  string
	}{
		{
			name:         "Numeric conversation (index mode)",
			agentName:    "test-agent",
			conversation: "1",
			wantPattern:  "---test-agent-",
		},
		{
			name:         "Custom conversation ID",
			agentName:    "test-agent",
			conversation: "my-session",
			wantPattern:  "my-session---test-agent-",
		},
		{
			name:         "Empty agent name defaults to 'default'",
			agentName:    "",
			conversation: "custom-id",
			wantPattern:  "custom-id---default-",
		},
		{
			name:         "Zero conversation (index mode)",
			agentName:    "agent",
			conversation: "0",
			wantPattern:  "---agent-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath := createNewHistoryFile(tempDir, tt.agentName, tt.conversation)
			
			// Extract filename from full path
			filename := filepath.Base(gotPath)
			
			// Check that the pattern exists in the filename
			if !filepath.IsAbs(gotPath) {
				t.Errorf("createNewHistoryFile() should return absolute path, got %v", gotPath)
			}
			
			if gotPath[:len(tempDir)] != tempDir {
				t.Errorf("createNewHistoryFile() should be in tempDir %v, got %v", tempDir, gotPath)
			}
			
			if !containsString(filename, tt.wantPattern) {
				t.Errorf("createNewHistoryFile() filename %v should contain pattern %v", filename, tt.wantPattern)
			}
			
			// Check that it ends with .json
			if filepath.Ext(filename) != ".json" {
				t.Errorf("createNewHistoryFile() should end with .json, got %v", filename)
			}
		})
	}
}

func TestFindHistoryFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create test history files
	testFiles := []string{
		"custom-session---test-agent---20240101-120000.json",
		"another-id---default---20240101-130000.json",
		"---old-agent---20240101-110000.json", // Index mode file (older)
		"---new-agent---20240101-140000.json", // Index mode file (newer)
	}

	for _, filename := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte("{}"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
		// Set different modification times to test sorting
		modTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		if filename == "---new-agent---20240101-140000.json" {
			modTime = time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC) // Newest
		} else if filename == "---old-agent---20240101-110000.json" {
			modTime = time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC) // Oldest
		}
		os.Chtimes(filePath, modTime, modTime)
	}

	tests := []struct {
		name         string
		conversation string
		wantFile     string
		wantError    bool
	}{
		{
			name:         "Find custom conversation ID",
			conversation: "custom-session",
			wantFile:     "custom-session---test-agent---20240101-120000.json",
			wantError:    false,
		},
		{
			name:         "Find another custom ID",
			conversation: "another-id",
			wantFile:     "another-id---default---20240101-130000.json",
			wantError:    false,
		},
		{
			name:         "Find by index 1 (newest)",
			conversation: "1",
			wantFile:     "---new-agent---20240101-140000.json",
			wantError:    false,
		},
		{
			name:         "Find by index 2",
			conversation: "2",
			wantFile:     "another-id---default---20240101-130000.json",
			wantError:    false,
		},
		{
			name:         "Nonexistent custom ID",
			conversation: "nonexistent",
			wantFile:     "",
			wantError:    true,
		},
		{
			name:         "Index out of range",
			conversation: "10",
			wantFile:     "",
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, err := findHistoryFile(tempDir, tt.conversation)
			
			if (err != nil) != tt.wantError {
				t.Errorf("findHistoryFile() error = %v, wantError %v", err, tt.wantError)
				return
			}
			
			if !tt.wantError {
				expectedPath := filepath.Join(tempDir, tt.wantFile)
				if gotPath != expectedPath {
					t.Errorf("findHistoryFile() = %v, want %v", gotPath, expectedPath)
				}
			}
		})
	}
}

func TestGetHistoryFilePath(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test history file
	testFile := "existing-session---test-agent---20240101-120000.json"
	filePath := filepath.Join(tempDir, testFile)
	if err := os.WriteFile(filePath, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name             string
		opts             *CLIOptions
		wantExists       bool
		wantPatternCheck func(string) bool
	}{
		{
			name: "New conversation with custom ID",
			opts: &CLIOptions{
				AgentName:    "test-agent",
				Conversation: "new-session",
				ContinueChat: false,
				RetryChat:    false,
			},
			wantExists: false,
			wantPatternCheck: func(path string) bool {
				filename := filepath.Base(path)
				return containsString(filename, "new-session---test-agent-")
			},
		},
		{
			name: "Continue existing conversation",
			opts: &CLIOptions{
				AgentName:    "test-agent",
				Conversation: "existing-session",
				ContinueChat: true,
				RetryChat:    false,
			},
			wantExists: true,
			wantPatternCheck: func(path string) bool {
				return path == filePath
			},
		},
		{
			name: "Retry existing conversation",
			opts: &CLIOptions{
				AgentName:    "test-agent",
				Conversation: "existing-session",
				ContinueChat: false,
				RetryChat:    true,
			},
			wantExists: true,
			wantPatternCheck: func(path string) bool {
				return path == filePath
			},
		},
		{
			name: "New conversation with numeric ID (index mode)",
			opts: &CLIOptions{
				AgentName:    "test-agent",
				Conversation: "1",
				ContinueChat: false,
				RetryChat:    false,
			},
			wantExists: false,
			wantPatternCheck: func(path string) bool {
				filename := filepath.Base(path)
				return containsString(filename, "---test-agent-")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotExists := getHistoryFilePath(tempDir, tt.opts)
			
			if gotExists != tt.wantExists {
				t.Errorf("getHistoryFilePath() exists = %v, want %v", gotExists, tt.wantExists)
			}
			
			if !tt.wantPatternCheck(gotPath) {
				t.Errorf("getHistoryFilePath() path %v does not match expected pattern", gotPath)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] != substr && 
		   (len(s) == len(substr) || s[:len(substr)] == substr || 
		    s[len(s)-len(substr):] == substr || 
		    findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}