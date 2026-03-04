package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/sashabaranov/go-openai"
)

const (
	anthropicAPIVersion    = "2023-06-01"
	anthropicDefaultMaxTok = 8192
)

// anthropicLLMClient implements LLMClient for the Anthropic Messages API.
type anthropicLLMClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func newAnthropicLLMClient(apiKey, baseURL string, httpClient *http.Client) LLMClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &anthropicLLMClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// -- Anthropic request types --

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []anthropicContentBlock
}

type anthropicContentBlock struct {
	Type      string  `json:"type"`
	Text      string  `json:"text,omitempty"`
	ID        string  `json:"id,omitempty"`
	Name      string  `json:"name,omitempty"`
	Input     any     `json:"input,omitempty"`
	ToolUseID string  `json:"tool_use_id,omitempty"`
	Content   *string `json:"content,omitempty"` // for tool_result text content (pointer to distinguish empty from absent)
	IsError   bool    `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

// -- Anthropic SSE event types --

type anthropicSSEEvent struct {
	Type string `json:"type"`
}

type anthropicContentBlockStart struct {
	Type         string                       `json:"type"`
	Index        int                          `json:"index"`
	ContentBlock anthropicContentBlockPartial `json:"content_block"`
}

type anthropicContentBlockPartial struct {
	Type  string `json:"type"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Text  string `json:"text,omitempty"`
	Input any    `json:"input,omitempty"`
}

type anthropicContentBlockDelta struct {
	Type  string                    `json:"type"`
	Index int                       `json:"index"`
	Delta anthropicContentDeltaBody `json:"delta"`
}

type anthropicContentDeltaBody struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type anthropicErrorEvent struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// -- Conversion helpers: openai messages → anthropic messages --

func convertOpenAIMessagesToAnthropic(messages []openai.ChatCompletionMessage) (string, []anthropicMessage) {
	var system string
	var anthropicMsgs []anthropicMessage

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			system = msg.Content
		case "user":
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    "user",
				Content: msg.Content,
			})
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls
				var blocks []anthropicContentBlock
				if msg.Content != "" {
					blocks = append(blocks, anthropicContentBlock{
						Type: "text",
						Text: msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					var input any
					if tc.Function.Arguments != "" {
						if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
							log.Printf("warning: failed to parse tool call arguments for %s: %v", tc.Function.Name, err)
						}
					}
					if input == nil {
						input = map[string]any{}
					}
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
				anthropicMsgs = append(anthropicMsgs, anthropicMessage{
					Role:    "assistant",
					Content: blocks,
				})
			} else {
				anthropicMsgs = append(anthropicMsgs, anthropicMessage{
					Role:    "assistant",
					Content: msg.Content,
				})
			}
		case "tool":
			// Anthropic expects tool results in a user message with tool_result content blocks.
			// Merge consecutive tool messages into a single user message.
			content := msg.Content
			block := anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   &content,
				IsError:   strings.HasPrefix(msg.Content, "Error: "),
			}
			// Check if the last message is already a user message with tool_result blocks
			if len(anthropicMsgs) > 0 {
				last := &anthropicMsgs[len(anthropicMsgs)-1]
				if last.Role == "user" {
					if blocks, ok := last.Content.([]anthropicContentBlock); ok {
						last.Content = append(blocks, block)
						continue
					}
				}
			}
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    "user",
				Content: []anthropicContentBlock{block},
			})
		}
	}

	return system, anthropicMsgs
}

func convertOpenAIToolsToAnthropic(tools []openai.Tool) []anthropicTool {
	var result []anthropicTool
	for _, t := range tools {
		if t.Function == nil {
			continue
		}
		result = append(result, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return result
}

// -- LLMClient implementation --

func (c *anthropicLLMClient) CreateChatCompletionStream(
	model string,
	messages []openai.ChatCompletionMessage,
	tools []openai.Tool,
) (LLMStream, error) {
	system, anthropicMsgs := convertOpenAIMessagesToAnthropic(messages)
	anthropicTools := convertOpenAIToolsToAnthropic(tools)

	reqBody := anthropicRequest{
		Model:     model,
		MaxTokens: anthropicDefaultMaxTok,
		System:    system,
		Messages:  anthropicMsgs,
		Tools:     anthropicTools,
		Stream:    true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	url := strings.TrimSuffix(c.baseURL, "/") + "/v1/messages"
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic API request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 429 {
			return nil, fmt.Errorf("429 Too Many Requests: %s", string(body))
		}
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	return &anthropicLLMStream{
		reader:         bufio.NewReader(resp.Body),
		body:           resp.Body,
		activeToolCall: nil,
		toolCallIndex:  0,
	}, nil
}

// -- Stream implementation --

type anthropicLLMStream struct {
	reader         *bufio.Reader
	body           io.ReadCloser
	done           bool
	activeToolCall *openai.ToolCall // Currently accumulating tool call
	toolCallIndex  int
}

func (s *anthropicLLMStream) Close() {
	if s.body != nil {
		s.body.Close()
	}
}

func (s *anthropicLLMStream) Recv() (LLMStreamDelta, error) {
	for {
		if s.done {
			return LLMStreamDelta{}, io.EOF
		}

		eventType, data, err := s.readSSEEvent()
		if err != nil {
			if err == io.EOF {
				s.done = true
				return LLMStreamDelta{}, io.EOF
			}
			return LLMStreamDelta{}, err
		}

		if eventType == "" || data == "" {
			continue
		}

		switch eventType {
		case "content_block_start":
			var event anthropicContentBlockStart
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				log.Printf("warning: failed to parse content_block_start: %v", err)
				continue
			}
			if event.ContentBlock.Type == "tool_use" {
				// Start a new tool call
				tc := openai.ToolCall{
					ID:   event.ContentBlock.ID,
					Type: "function",
					Function: openai.FunctionCall{
						Name:      event.ContentBlock.Name,
						Arguments: "",
					},
				}
				s.activeToolCall = &tc
				s.toolCallIndex = event.Index
				return LLMStreamDelta{
					ToolCalls: []openai.ToolCall{tc},
				}, nil
			}
			// text block start — no content yet
			continue

		case "content_block_delta":
			var event anthropicContentBlockDelta
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				log.Printf("warning: failed to parse content_block_delta: %v", err)
				continue
			}
			switch event.Delta.Type {
			case "text_delta":
				return LLMStreamDelta{
					Content: event.Delta.Text,
				}, nil
			case "input_json_delta":
				// Append to active tool call arguments
				if s.activeToolCall != nil {
					// Return a delta with empty ID to signal argument continuation
					return LLMStreamDelta{
						ToolCalls: []openai.ToolCall{{
							Function: openai.FunctionCall{
								Arguments: event.Delta.PartialJSON,
							},
						}},
					}, nil
				}
			}
			continue

		case "content_block_stop":
			s.activeToolCall = nil
			continue

		case "message_stop":
			s.done = true
			return LLMStreamDelta{}, io.EOF

		case "message_delta":
			// May contain stop_reason, usage info — ignore for streaming
			continue

		case "message_start":
			// Initial message metadata — ignore for streaming
			continue

		case "ping":
			continue

		case "error":
			var errEvent anthropicErrorEvent
			if err := json.Unmarshal([]byte(data), &errEvent); err != nil {
				return LLMStreamDelta{}, fmt.Errorf("anthropic stream error: %s", data)
			}
			return LLMStreamDelta{}, fmt.Errorf("anthropic stream error: %s: %s",
				errEvent.Error.Type, errEvent.Error.Message)
		}
	}
}

// readSSEEvent reads a single SSE event from the stream.
// Returns the event type and data payload.
func (s *anthropicLLMStream) readSSEEvent() (string, string, error) {
	var eventType string
	var dataLines []string

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && line != "" {
				// Process the last line even if no trailing newline
			} else if err == io.EOF {
				if eventType != "" || len(dataLines) > 0 {
					return eventType, strings.Join(dataLines, "\n"), nil
				}
				return "", "", io.EOF
			} else {
				return "", "", err
			}
		}

		line = strings.TrimRight(line, "\r\n")

		// Empty line marks end of event
		if line == "" {
			if eventType != "" || len(dataLines) > 0 {
				return eventType, strings.Join(dataLines, "\n"), nil
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
		// Ignore other fields like id:, retry:, comments (:)
	}
}

var _ LLMClient = (*anthropicLLMClient)(nil)
var _ LLMStream = (*anthropicLLMStream)(nil)
