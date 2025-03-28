package main

import "testing"

func TestParseModel(t *testing.T) {
	tests := []struct {
		name         string
		modelFlag    string
		config       *Config
		wantProvider string
		wantModel    string
		wantInfo     providerInfo
	}{
		{
			name:         "OpenAI default model",
			modelFlag:    "openai/gpt-4",
			config:       nil,
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
			wantProvider: "ollama",
			wantModel:    "llama2",
			wantInfo: providerInfo{
				baseURL:     "http://localhost:11434/v1",
				apiKeyEnvar: "ESA_API_KEY",
			},
		},
		{
			name:         "No provider specified",
			modelFlag:    "gpt-4",
			config:       nil,
			wantProvider: "openai",
			wantModel:    "gpt-4",
			wantInfo: providerInfo{
				baseURL:     "https://api.openai.com/v1",
				apiKeyEnvar: "OPENAI_API_KEY",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &Application{
				modelFlag: tt.modelFlag,
				config:    tt.config,
			}
			gotProvider, gotModel, gotInfo := app.parseModel()
			if gotProvider != tt.wantProvider {
				t.Errorf("parseModel() provider = %v, want %v", gotProvider, tt.wantProvider)
			}
			if gotModel != tt.wantModel {
				t.Errorf("parseModel() model = %v, want %v", gotModel, tt.wantModel)
			}
			if gotInfo != tt.wantInfo {
				t.Errorf("parseModel() info = %+v, want %+v", gotInfo, tt.wantInfo)
			}
		})
	}
}
