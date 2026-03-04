package main

import (
	"encoding/json"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestConvertOpenAIMessagesToAnthropic(t *testing.T) {
	tests := []struct {
		name          string
		messages      []openai.ChatCompletionMessage
		wantSystem    string
		wantMsgCount  int
		wantFirstRole string
	}{
		{
			name: "system message extracted",
			messages: []openai.ChatCompletionMessage{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "Hello"},
			},
			wantSystem:    "You are helpful",
			wantMsgCount:  1,
			wantFirstRole: "user",
		},
		{
			name: "user and assistant messages",
			messages: []openai.ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there"},
			},
			wantSystem:    "",
			wantMsgCount:  2,
			wantFirstRole: "user",
		},
		{
			name: "tool result becomes user message",
			messages: []openai.ChatCompletionMessage{
				{Role: "user", Content: "Run the command"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []openai.ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: openai.FunctionCall{
								Name:      "run_cmd",
								Arguments: `{"cmd":"ls"}`,
							},
						},
					},
				},
				{Role: "tool", Content: "file1.txt\nfile2.txt", ToolCallID: "call_123"},
			},
			wantSystem:    "",
			wantMsgCount:  3,
			wantFirstRole: "user",
		},
		{
			name: "consecutive tool results merged into single user message",
			messages: []openai.ChatCompletionMessage{
				{Role: "user", Content: "Run commands"},
				{
					Role: "assistant",
					ToolCalls: []openai.ToolCall{
						{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "cmd1", Arguments: "{}"}},
						{ID: "call_2", Type: "function", Function: openai.FunctionCall{Name: "cmd2", Arguments: "{}"}},
					},
				},
				{Role: "tool", Content: "result1", ToolCallID: "call_1"},
				{Role: "tool", Content: "result2", ToolCallID: "call_2"},
			},
			wantSystem:   "",
			wantMsgCount: 3, // user, assistant, user(with 2 tool_result blocks)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			system, msgs := convertOpenAIMessagesToAnthropic(tt.messages)

			if system != tt.wantSystem {
				t.Errorf("system = %q, want %q", system, tt.wantSystem)
			}

			if len(msgs) != tt.wantMsgCount {
				t.Errorf("message count = %d, want %d", len(msgs), tt.wantMsgCount)
			}

			if tt.wantFirstRole != "" && len(msgs) > 0 {
				if msgs[0].Role != tt.wantFirstRole {
					t.Errorf("first message role = %q, want %q", msgs[0].Role, tt.wantFirstRole)
				}
			}
		})
	}
}

func TestConvertOpenAIToolsToAnthropic(t *testing.T) {
	tests := []struct {
		name      string
		tools     []openai.Tool
		wantCount int
		wantNames []string
	}{
		{
			name:      "empty tools",
			tools:     nil,
			wantCount: 0,
		},
		{
			name: "single tool conversion",
			tools: []openai.Tool{
				{
					Type: openai.ToolTypeFunction,
					Function: &openai.FunctionDefinition{
						Name:        "get_weather",
						Description: "Get weather info",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"city": map[string]any{
									"type":        "string",
									"description": "City name",
								},
							},
							"required": []string{"city"},
						},
					},
				},
			},
			wantCount: 1,
			wantNames: []string{"get_weather"},
		},
		{
			name: "nil function is skipped",
			tools: []openai.Tool{
				{Type: openai.ToolTypeFunction, Function: nil},
				{
					Type: openai.ToolTypeFunction,
					Function: &openai.FunctionDefinition{
						Name:        "valid_tool",
						Description: "A valid tool",
						Parameters:  map[string]any{"type": "object"},
					},
				},
			},
			wantCount: 1,
			wantNames: []string{"valid_tool"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertOpenAIToolsToAnthropic(tt.tools)

			if len(result) != tt.wantCount {
				t.Errorf("tool count = %d, want %d", len(result), tt.wantCount)
			}

			for i, wantName := range tt.wantNames {
				if i < len(result) && result[i].Name != wantName {
					t.Errorf("tool[%d].Name = %q, want %q", i, result[i].Name, wantName)
				}
			}
		})
	}
}

func TestConvertAssistantToolCallMessage(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "use tool"},
		{
			Role: "assistant",
			ToolCalls: []openai.ToolCall{
				{
					ID:   "tc_1",
					Type: "function",
					Function: openai.FunctionCall{
						Name:      "my_func",
						Arguments: `{"key":"value"}`,
					},
				},
			},
		},
	}

	system, msgs := convertOpenAIMessagesToAnthropic(messages)

	if system != "system prompt" {
		t.Errorf("system = %q, want %q", system, "system prompt")
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// The assistant message should have content blocks
	assistantMsg := msgs[1]
	if assistantMsg.Role != "assistant" {
		t.Errorf("expected assistant role, got %q", assistantMsg.Role)
	}

	// The content should be an array of content blocks
	blocks, ok := assistantMsg.Content.([]anthropicContentBlock)
	if !ok {
		t.Fatalf("expected []anthropicContentBlock, got %T", assistantMsg.Content)
	}

	if len(blocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(blocks))
	}

	if blocks[0].Type != "tool_use" {
		t.Errorf("block type = %q, want %q", blocks[0].Type, "tool_use")
	}
	if blocks[0].ID != "tc_1" {
		t.Errorf("block ID = %q, want %q", blocks[0].ID, "tc_1")
	}
	if blocks[0].Name != "my_func" {
		t.Errorf("block Name = %q, want %q", blocks[0].Name, "my_func")
	}

	// Check that Input was parsed from JSON
	inputJSON, err := json.Marshal(blocks[0].Input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}
	if string(inputJSON) != `{"key":"value"}` {
		t.Errorf("block Input = %s, want %s", string(inputJSON), `{"key":"value"}`)
	}
}
