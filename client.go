package main

import (
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"net/http"
	"os"
	"time"

	"github.com/sashabaranov/go-openai"
)

// setupLLMClient creates the appropriate LLMClient for the given model/provider.
// For the "anthropic" provider it returns a native Anthropic client;
// for all other providers it returns an OpenAI-compatible client.
func setupLLMClient(modelStr string, agent Agent, config *Config) (LLMClient, error) {
	provider, _, info := parseModel(modelStr, agent, config)

	configuredAPIKey := os.Getenv(info.apiKeyEnvar)
	// Key name can be empty if we don't need any keys
	if info.apiKeyEnvar != "" && configuredAPIKey == "" && !info.apiKeyCanBeEmpty {
		return nil, fmt.Errorf(info.apiKeyEnvar + " env not found")
	}

	if provider == "anthropic" {
		var httpClient *http.Client
		if len(info.additionalHeaders) != 0 {
			httpClient = &http.Client{
				Transport: &transportWithCustomHeaders{
					headers: info.additionalHeaders,
					base:    http.DefaultTransport,
				},
			}
		}
		return newAnthropicLLMClient(configuredAPIKey, info.baseURL, httpClient), nil
	}

	// Default: OpenAI-compatible provider
	return setupOpenAIClient(configuredAPIKey, info)
}

func setupOpenAIClient(apiKey string, info providerInfo) (LLMClient, error) {
	llmConfig := openai.DefaultConfig(apiKey)
	llmConfig.BaseURL = info.baseURL

	if len(info.additionalHeaders) != 0 {
		httpClient := &http.Client{
			Transport: &transportWithCustomHeaders{
				headers: info.additionalHeaders,
				base:    http.DefaultTransport,
			},
		}

		llmConfig.HTTPClient = httpClient
	}

	client := openai.NewClientWithConfig(llmConfig)

	return newOpenAILLMClient(client), nil
}

type transportWithCustomHeaders struct {
	headers map[string]string
	base    http.RoundTripper
}

func (t *transportWithCustomHeaders) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	return t.base.RoundTrip(req)
}

// calculateRetryDelay calculates exponential backoff delay with jitter
func calculateRetryDelay(attempt int) time.Duration {
	// Exponential backoff: baseDelay * 2^attempt
	// Cap in float64 space to avoid int64 overflow for large attempt values
	delayF := float64(baseRetryDelay) * math.Pow(2, float64(attempt))
	if delayF > float64(maxRetryDelay) || delayF < 0 {
		delayF = float64(maxRetryDelay)
	}
	delay := time.Duration(delayF)

	// Add jitter: random value in [0, delay/2)
	half := int64(delay / 2)
	if half <= 0 {
		return delay
	}
	jitter := time.Duration(mathrand.Int64N(half))
	return delay + jitter
}
