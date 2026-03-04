package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
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

func TestStreamParseToolUseContentBlockStart(t *testing.T) {
	// Simulate the SSE events Anthropic sends for a tool_use response.
	// The key issue was that content_block_start sends "input": {} (an object),
	// which failed to parse when Input was typed as string.
	sseData := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"I'll calculate that."}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":0}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01A","name":"calculate","input":{}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"express"}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"ion\": \"2+2\"}"}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":1}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	stream := &anthropicLLMStream{
		reader: newBufioReader(sseData),
		body:   io.NopCloser(strings.NewReader("")),
	}

	var textContent string
	var toolCalls []openai.ToolCall

	for {
		delta, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}

		if delta.Content != "" {
			textContent += delta.Content
		}

		for _, tc := range delta.ToolCalls {
			if tc.ID != "" {
				// New tool call
				toolCalls = append(toolCalls, tc)
			} else {
				// Argument continuation
				if len(toolCalls) == 0 {
					t.Fatal("got argument delta before any tool call started")
				}
				toolCalls[len(toolCalls)-1].Function.Arguments += tc.Function.Arguments
			}
		}
	}

	if textContent != "I'll calculate that." {
		t.Errorf("text content = %q, want %q", textContent, "I'll calculate that.")
	}

	if len(toolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(toolCalls))
	}

	tc := toolCalls[0]
	if tc.ID != "toolu_01A" {
		t.Errorf("tool call ID = %q, want %q", tc.ID, "toolu_01A")
	}
	if tc.Function.Name != "calculate" {
		t.Errorf("tool call name = %q, want %q", tc.Function.Name, "calculate")
	}
	if tc.Function.Arguments != `{"expression": "2+2"}` {
		t.Errorf("tool call arguments = %q, want %q", tc.Function.Arguments, `{"expression": "2+2"}`)
	}
}

func TestToolResultSerialization(t *testing.T) {
	tests := []struct {
		name           string
		messages       []openai.ChatCompletionMessage
		wantContentKey bool // whether the tool_result should have a "content" key
		wantIsError    bool
	}{
		{
			name: "tool result with content",
			messages: []openai.ChatCompletionMessage{
				{Role: "user", Content: "do something"},
				{
					Role: "assistant",
					ToolCalls: []openai.ToolCall{
						{ID: "tc_1", Type: "function", Function: openai.FunctionCall{Name: "cmd", Arguments: "{}"}},
					},
				},
				{Role: "tool", Content: "output text", ToolCallID: "tc_1"},
			},
			wantContentKey: true,
			wantIsError:    false,
		},
		{
			name: "tool result with empty content still serialized",
			messages: []openai.ChatCompletionMessage{
				{Role: "user", Content: "do something"},
				{
					Role: "assistant",
					ToolCalls: []openai.ToolCall{
						{ID: "tc_1", Type: "function", Function: openai.FunctionCall{Name: "cmd", Arguments: "{}"}},
					},
				},
				{Role: "tool", Content: "", ToolCallID: "tc_1"},
			},
			wantContentKey: true,
			wantIsError:    false,
		},
		{
			name: "tool result with error sets is_error",
			messages: []openai.ChatCompletionMessage{
				{Role: "user", Content: "do something"},
				{
					Role: "assistant",
					ToolCalls: []openai.ToolCall{
						{ID: "tc_1", Type: "function", Function: openai.FunctionCall{Name: "cmd", Arguments: "{}"}},
					},
				},
				{Role: "tool", Content: "Error: command failed", ToolCallID: "tc_1"},
			},
			wantContentKey: true,
			wantIsError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, msgs := convertOpenAIMessagesToAnthropic(tt.messages)

			// The last message should be a user message with tool_result blocks
			lastMsg := msgs[len(msgs)-1]
			if lastMsg.Role != "user" {
				t.Fatalf("last message role = %q, want %q", lastMsg.Role, "user")
			}

			blocks, ok := lastMsg.Content.([]anthropicContentBlock)
			if !ok {
				t.Fatalf("expected []anthropicContentBlock, got %T", lastMsg.Content)
			}

			if len(blocks) != 1 {
				t.Fatalf("expected 1 block, got %d", len(blocks))
			}

			block := blocks[0]
			if block.Type != "tool_result" {
				t.Errorf("block type = %q, want %q", block.Type, "tool_result")
			}

			// Serialize to JSON and check the content key presence
			jsonData, err := json.Marshal(block)
			if err != nil {
				t.Fatalf("failed to marshal block: %v", err)
			}

			var raw map[string]any
			json.Unmarshal(jsonData, &raw)

			_, hasContent := raw["content"]
			if hasContent != tt.wantContentKey {
				t.Errorf("content key present = %v, want %v (json: %s)", hasContent, tt.wantContentKey, string(jsonData))
			}

			if tt.wantIsError {
				isError, hasIsError := raw["is_error"]
				if !hasIsError || isError != true {
					t.Errorf("expected is_error=true in json, got %s", string(jsonData))
				}
			} else {
				if _, hasIsError := raw["is_error"]; hasIsError {
					t.Errorf("expected no is_error in json, got %s", string(jsonData))
				}
			}
		})
	}
}

func TestStreamParseMultipleToolCalls(t *testing.T) {
	// Test that multiple tool_use blocks in a single response are parsed correctly
	sseData := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[]}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"cmd1","input":{}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"a\":\"1\"}"}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":0}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_02","name":"cmd2","input":{}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"b\":\"2\"}"}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":1}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	stream := &anthropicLLMStream{
		reader: newBufioReader(sseData),
		body:   io.NopCloser(strings.NewReader("")),
	}

	var toolCalls []openai.ToolCall

	for {
		delta, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}

		for _, tc := range delta.ToolCalls {
			if tc.ID != "" {
				toolCalls = append(toolCalls, tc)
			} else {
				toolCalls[len(toolCalls)-1].Function.Arguments += tc.Function.Arguments
			}
		}
	}

	if len(toolCalls) != 2 {
		t.Fatalf("tool call count = %d, want 2", len(toolCalls))
	}

	if toolCalls[0].ID != "toolu_01" {
		t.Errorf("first tool call ID = %q, want %q", toolCalls[0].ID, "toolu_01")
	}
	if toolCalls[0].Function.Name != "cmd1" {
		t.Errorf("first tool call name = %q, want %q", toolCalls[0].Function.Name, "cmd1")
	}
	if toolCalls[0].Function.Arguments != `{"a":"1"}` {
		t.Errorf("first tool call args = %q, want %q", toolCalls[0].Function.Arguments, `{"a":"1"}`)
	}

	if toolCalls[1].ID != "toolu_02" {
		t.Errorf("second tool call ID = %q, want %q", toolCalls[1].ID, "toolu_02")
	}
	if toolCalls[1].Function.Name != "cmd2" {
		t.Errorf("second tool call name = %q, want %q", toolCalls[1].Function.Name, "cmd2")
	}
	if toolCalls[1].Function.Arguments != `{"b":"2"}` {
		t.Errorf("second tool call args = %q, want %q", toolCalls[1].Function.Arguments, `{"b":"2"}`)
	}
}

// newBufioReader creates a bufio.Reader from a string for testing
func newBufioReader(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s))
}
