package main

import (
	"os"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestApplication_ProcessInput(t *testing.T) {
	tests := []struct {
		name         string
		config       Config
		commandStr   string
		stdinContent string
		expectedMsgs []openai.ChatCompletionMessage
	}{
		{
			name: "initial message from config without other input",
			config: Config{
				SystemPrompt:   "test system prompt",
				InitialMessage: "test initial message",
			},
			commandStr:   "",
			stdinContent: "",
			expectedMsgs: []openai.ChatCompletionMessage{
				{Role: "system", Content: "test system prompt"},
				{Role: "user", Content: "test initial message"},
			},
		},
		{
			name: "command string should override initial message",
			config: Config{
				SystemPrompt:   "test system prompt",
				InitialMessage: "test initial message",
			},
			commandStr:   "command input",
			stdinContent: "",
			expectedMsgs: []openai.ChatCompletionMessage{
				{Role: "system", Content: "test system prompt"},
				{Role: "user", Content: "command input"},
			},
		},
		{
			name: "stdin should override initial message",
			config: Config{
				SystemPrompt:   "test system prompt",
				InitialMessage: "test initial message",
			},
			commandStr:   "",
			stdinContent: "stdin input",
			expectedMsgs: []openai.ChatCompletionMessage{
				{Role: "system", Content: "test system prompt"},
				{Role: "user", Content: "stdin input"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup stdin if needed
			if tt.stdinContent != "" {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				oldStdin := os.Stdin
				os.Stdin = r
				w.Write([]byte(tt.stdinContent))
				w.Close()
				defer func() { os.Stdin = oldStdin }()
			}

			app := &Application{
				config:   tt.config,
				messages: []openai.ChatCompletionMessage{{Role: "system", Content: tt.config.SystemPrompt}},
			}

			app.processInput(tt.commandStr)

			if len(app.messages) != len(tt.expectedMsgs) {
				t.Errorf("Expected %d messages, got %d", len(tt.expectedMsgs), len(app.messages))
				return
			}

			for i, msg := range app.messages {
				if msg.Role != tt.expectedMsgs[i].Role {
					t.Errorf("Message %d: expected role %s, got %s", i, tt.expectedMsgs[i].Role, msg.Role)
				}
				if msg.Content != tt.expectedMsgs[i].Content {
					t.Errorf("Message %d: expected content %q, got %q", i, tt.expectedMsgs[i].Content, msg.Content)
				}
			}
		})
	}
}

func TestApplication_ProcessInitialMessage(t *testing.T) {
	tests := []struct {
		name            string
		initialMessage  string
		expectedContent string
	}{
		{
			name:            "basic message without eval",
			initialMessage:  "Hello, how can I help?",
			expectedContent: "Hello, how can I help?",
		},
		{
			name:            "message with eval block",
			initialMessage:  "Current dir: {{$pwd}}",
			expectedContent: "Current dir: " + os.Getenv("PWD"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &Application{}
			result := app.processInitialMessage(tt.initialMessage)
			if result != tt.expectedContent {
				t.Errorf("Expected content %q, got %q", tt.expectedContent, result)
			}
		})
	}
}
