package main

import (
	"testing"
)

func TestParseModel(t *testing.T) {
	tests := []struct {
		name         string
		modelFlag    string
		config       *Config
		agent        Agent
		wantProvider string
		wantModel    string
		wantInfo     providerInfo
	}{
		{
			name:         "OpenAI default model",
			modelFlag:    "openai/gpt-4",
			config:       nil,
			agent:        Agent{},
			wantProvider: "openai",
			wantModel:    "gpt-4",
			wantInfo: providerInfo{
				baseURL:     "https://api.openai.com/v1",
				apiKeyEnvar: "OPENAI_API_KEY",
			},
		},
		{
			name:      "Custom provider from config",
			modelFlag: "custom/model-1",
			config: &Config{
				Providers: map[string]ProviderConfig{
					"custom": {
						BaseURL:     "https://custom.api/v1",
						APIKeyEnvar: "CUSTOM_API_KEY",
					},
				},
			},
			agent:        Agent{},
			wantProvider: "custom",
			wantModel:    "model-1",
			wantInfo: providerInfo{
				baseURL:     "https://custom.api/v1",
				apiKeyEnvar: "CUSTOM_API_KEY",
			},
		},
		{
			name:      "Partial provider override in config",
			modelFlag: "openai/gpt-4",
			config: &Config{
				Providers: map[string]ProviderConfig{
					"openai": {
						BaseURL: "https://custom-openai.api/v2",
						// APIKeyEnvar not specified, should use default
					},
				},
			},
			agent:        Agent{},
			wantProvider: "openai",
			wantModel:    "gpt-4",
			wantInfo: providerInfo{
				baseURL:     "https://custom-openai.api/v2",
				apiKeyEnvar: "OPENAI_API_KEY", // Should keep default
			},
		},
		{
			name:      "Custom provider with builtin still available",
			modelFlag: "custom/model-1",
			config: &Config{
				Providers: map[string]ProviderConfig{
					"custom": {
						BaseURL:     "https://custom.api/v1",
						APIKeyEnvar: "CUSTOM_API_KEY",
					},
				},
			},
			agent:        Agent{},
			wantProvider: "custom",
			wantModel:    "model-1",
			wantInfo: providerInfo{
				baseURL:     "https://custom.api/v1",
				apiKeyEnvar: "CUSTOM_API_KEY",
			},
		},
		{
			name:         "Built-in provider unchanged",
			modelFlag:    "ollama/llama2",
			config:       &Config{}, // Empty config
			agent:        Agent{},
			wantProvider: "ollama",
			wantModel:    "llama2",
			wantInfo: providerInfo{
				baseURL:     "http://localhost:11434/v1",
				apiKeyEnvar: "OLLAMA_API_KEY",
			},
		},
		{
			name:      "Agent default model used when no CLI model provided",
			modelFlag: "",
			config:    nil,
			agent: Agent{
				DefaultModel: "groq/llama3-8b",
			},
			wantProvider: "groq",
			wantModel:    "llama3-8b",
			wantInfo: providerInfo{
				baseURL:     "https://api.groq.com/openai/v1",
				apiKeyEnvar: "GROQ_API_KEY",
			},
		},
		{
			name:      "CLI model overrides agent default model",
			modelFlag: "openai/gpt-4",
			config:    nil,
			agent: Agent{
				DefaultModel: "groq/llama3-8b",
			},
			wantProvider: "openai",
			wantModel:    "gpt-4",
			wantInfo: providerInfo{
				baseURL:     "https://api.openai.com/v1",
				apiKeyEnvar: "OPENAI_API_KEY",
			},
		},
		{
			name:      "Agent default model overrides config default model",
			modelFlag: "",
			config: &Config{
				Settings: Settings{
					DefaultModel: "ollama/codellama",
				},
			},
			agent: Agent{
				DefaultModel: "groq/llama3-8b",
			},
			wantProvider: "groq",
			wantModel:    "llama3-8b",
			wantInfo: providerInfo{
				baseURL:     "https://api.groq.com/openai/v1",
				apiKeyEnvar: "GROQ_API_KEY",
			},
		},
		{
			name:      "Agent default model overrides global fallback",
			modelFlag: "",
			config:    nil,
			agent: Agent{
				DefaultModel: "ollama/llama3.2",
			},
			wantProvider: "ollama",
			wantModel:    "llama3.2",
			wantInfo: providerInfo{
				baseURL:     "http://localhost:11434/v1",
				apiKeyEnvar: "OLLAMA_API_KEY",
			},
		},
		{
			name:      "Config default model used when agent has no default",
			modelFlag: "",
			config: &Config{
				Settings: Settings{
					DefaultModel: "ollama/codellama",
				},
			},
			agent:        Agent{}, // No default model
			wantProvider: "ollama",
			wantModel:    "codellama",
			wantInfo: providerInfo{
				baseURL:     "http://localhost:11434/v1",
				apiKeyEnvar: "OLLAMA_API_KEY",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &Application{
				modelFlag: tt.modelFlag,
				config:    tt.config,
				agent:     tt.agent,
			}
			gotProvider, gotModel, gotInfo := app.parseModel()
			if gotProvider != tt.wantProvider {
				t.Errorf("parseModel() provider = %v, want %v", gotProvider, tt.wantProvider)
			}
			if gotModel != tt.wantModel {
				t.Errorf("parseModel() model = %v, want %v", gotModel, tt.wantModel)
			}
			if gotInfo.apiKeyEnvar != tt.wantInfo.apiKeyEnvar {
				t.Errorf("parseModel() info.apiKeyEnvar = %+v, want %+v", gotInfo.apiKeyEnvar, tt.wantInfo.apiKeyEnvar)
			}
			if gotInfo.baseURL != tt.wantInfo.baseURL {
				t.Errorf("parseModel() info.baseURL = %+v, want %+v", gotInfo.baseURL, tt.wantInfo.baseURL)
			}
			if gotInfo.baseURL == "" {
				t.Errorf("parseModel() info baseURL should not be empty")
			}
			// Improved additionalHeaders test: check keys and values
			if len(gotInfo.additionalHeaders) != len(tt.wantInfo.additionalHeaders) {
				t.Errorf("parseModel() info.additionalHeaders len = %d, want %d", len(gotInfo.additionalHeaders), len(tt.wantInfo.additionalHeaders))
			}
			for k, v := range tt.wantInfo.additionalHeaders {
				gotVal, ok := gotInfo.additionalHeaders[k]
				if !ok {
					t.Errorf("parseModel() info.additionalHeaders missing key %q", k)
				} else if gotVal != v {
					t.Errorf("parseModel() info.additionalHeaders[%q] = %q, want %q", k, gotVal, v)
				}
			}
			for k := range gotInfo.additionalHeaders {
				if _, ok := tt.wantInfo.additionalHeaders[k]; !ok {
					t.Errorf("parseModel() info.additionalHeaders has unexpected key %q", k)
				}
			}
		})
	}

}

func TestProviderAdditionalHeadersMerging(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"copilot": {
				AdditionalHeaders: map[string]string{
					"Copilot-Integration-Id": "custom-override",
					"X-Extra":                "foo",
				},
			},
		},
	}
	app := &Application{
		modelFlag: "copilot/some-model",
		config:    cfg,
		agent:     Agent{},
	}
	_, _, info := app.parseModel()
	expected := map[string]string{
		"Content-Type":           "application/json",
		"Copilot-Integration-Id": "custom-override", // overridden
		"X-Extra":                "foo",
	}
	if len(info.additionalHeaders) != len(expected) {
		t.Errorf("additionalHeaders len = %d, want %d", len(info.additionalHeaders), len(expected))
	}
	for k, v := range expected {
		got, ok := info.additionalHeaders[k]
		if !ok {
			t.Errorf("missing additionalHeader %q", k)
		} else if got != v {
			t.Errorf("additionalHeader[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestOllamaHostFromEnvironment(t *testing.T) {
	tests := []struct {
		name      string
		hostEnv   string
		wantHost  string
		needsTrim bool
	}{
		{
			name:     "Default host when env not set",
			hostEnv:  "",
			wantHost: "http://localhost:11434/v1",
		},
		{
			name:      "Custom host from environment",
			hostEnv:   "http://192.168.1.100:11434",
			wantHost:  "http://192.168.1.100:11434/v1",
			needsTrim: false,
		},
		{
			name:      "Custom host with trailing slash",
			hostEnv:   "http://ollama-server:11434/",
			wantHost:  "http://ollama-server:11434/v1",
			needsTrim: true,
		},
		{
			name:      "Host with existing /v1 path",
			hostEnv:   "http://custom-ollama:8000/v1",
			wantHost:  "http://custom-ollama:8000/v1",
			needsTrim: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OLLAMA_HOST", tt.hostEnv)

			app := &Application{modelFlag: "ollama/llama2"}

			_, _, info := app.parseModel()

			if info.baseURL != tt.wantHost {
				t.Errorf("Expected Ollama host %q, got %q", tt.wantHost, info.baseURL)
			}
		})
	}
}

func TestEmptyApiKeyAcceptance(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	tests := []struct {
		name             string
		modelStr         string
		expectError      bool
		errorDescription string
	}{
		{
			name:             "Ollama accepts empty API key",
			modelStr:         "ollama/llama2",
			expectError:      false,
			errorDescription: "",
		},
		{
			name:             "OpenAI requires API key",
			modelStr:         "openai/gpt-4",
			expectError:      true,
			errorDescription: "OPENAI_API_KEY env not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := setupOpenAIClient(tt.modelStr, Agent{}, &Config{})

			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, err)
			}

			if tt.expectError && err != nil && err.Error() != tt.errorDescription {
				t.Errorf("Expected error message %q, got %q", tt.errorDescription, err.Error())
			}
		})
	}
}

func TestSystemPromptOverrideFromCLI(t *testing.T) {
	// Agent with default system prompt
	agent := Agent{
		SystemPrompt: "Default system prompt",
	}
	opts := &CLIOptions{
		SystemPrompt: "CLI system prompt override",
	}
	app := &Application{
		agent: agent,
	}
	// Simulate NewApplication logic
	if opts.SystemPrompt != "" {
		app.agent.SystemPrompt = opts.SystemPrompt
	}
	prompt, err := app.getSystemPrompt()
	if err != nil {
		t.Fatalf("getSystemPrompt error: %v", err)
	}
	if prompt != "CLI system prompt override" {
		t.Errorf("Expected system prompt to be overridden by CLI, got: %q", prompt)
	}
}
