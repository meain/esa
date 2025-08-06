package main

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/sashabaranov/go-openai"
)

func setupOpenAIClient(modelStr string, agent Agent, config *Config) (*openai.Client, error) {
	_, _, info := parseModel(modelStr, agent, config)

	configuredAPIKey := os.Getenv(info.apiKeyEnvar)
	// Key name can be empty if we don't need any keys
	if info.apiKeyEnvar != "" && configuredAPIKey == "" && !info.apiKeyCanBeEmpty {
		return nil, fmt.Errorf(info.apiKeyEnvar + " env not found")
	}

	llmConfig := openai.DefaultConfig(configuredAPIKey)
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

	return client, nil
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
	delay := time.Duration(float64(baseRetryDelay) * math.Pow(2, float64(attempt)))

	// Cap the delay
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}

	return delay
}
