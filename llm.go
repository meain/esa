package main

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// LLMStream represents a streaming response from an LLM provider.
// It yields deltas that may contain text content or tool call fragments.
type LLMStream interface {
	// Recv returns the next streaming delta. Returns io.EOF when the stream is complete.
	Recv() (LLMStreamDelta, error)
	// Close closes the stream.
	Close()
}

// LLMStreamDelta represents a single chunk from a streaming response.
type LLMStreamDelta struct {
	// Content is a text fragment from the assistant's response.
	Content string
	// ToolCalls contains tool call fragments being streamed.
	// A tool call with a non-empty ID signals a new tool call;
	// subsequent deltas with empty ID append to the last tool call's arguments.
	ToolCalls []openai.ToolCall
}

// LLMClient abstracts an LLM provider for creating streaming chat completions.
type LLMClient interface {
	// CreateChatCompletionStream starts a streaming chat completion.
	CreateChatCompletionStream(
		model string,
		messages []openai.ChatCompletionMessage,
		tools []openai.Tool,
	) (LLMStream, error)
}

// openAILLMClient wraps the go-openai client to implement LLMClient.
type openAILLMClient struct {
	client *openai.Client
}

func newOpenAILLMClient(client *openai.Client) LLMClient {
	return &openAILLMClient{client: client}
}

func (c *openAILLMClient) CreateChatCompletionStream(
	model string,
	messages []openai.ChatCompletionMessage,
	tools []openai.Tool,
) (LLMStream, error) {
	stream, err := c.client.CreateChatCompletionStream(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    model,
			Messages: messages,
			Tools:    tools,
		})
	if err != nil {
		return nil, err
	}
	return &openAILLMStream{stream: stream}, nil
}

// openAILLMStream wraps the go-openai stream to implement LLMStream.
type openAILLMStream struct {
	stream *openai.ChatCompletionStream
}

func (s *openAILLMStream) Recv() (LLMStreamDelta, error) {
	response, err := s.stream.Recv()
	if err != nil {
		return LLMStreamDelta{}, err
	}

	if len(response.Choices) == 0 {
		return LLMStreamDelta{}, nil
	}

	delta := LLMStreamDelta{
		Content:   response.Choices[0].Delta.Content,
		ToolCalls: response.Choices[0].Delta.ToolCalls,
	}
	return delta, nil
}

func (s *openAILLMStream) Close() {
	s.stream.Close()
}

var _ LLMClient = (*openAILLMClient)(nil)
var _ LLMStream = (*openAILLMStream)(nil)
