package main

import "testing"

func TestParseModel(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantProvider string
		wantModel    string
		wantInfo     providerInfo
	}{
		{
			name:         "OpenAI model",
			input:        "openai/gpt-4",
			wantProvider: "openai",
			wantModel:    "gpt-4",
			wantInfo: providerInfo{
				baseURL:     "https://api.openai.com/v1",
				apiKeyEnvar: "OPENAI_API_KEY",
			},
		},
		{
			name:         "Anthropic model",
			input:        "anthropic/claude-2",
			wantProvider: "anthropic",
			wantModel:    "claude-2",
			wantInfo: providerInfo{
				baseURL:     "https://api.anthropic.com/v1",
				apiKeyEnvar: "ANTHROPIC_API_KEY",
			},
		},
		{
			name:         "Azure model",
			input:        "azure/gpt-4",
			wantProvider: "azure",
			wantModel:    "gpt-4",
			wantInfo: providerInfo{
				baseURL:     "https://api.azure.com/v1",
				apiKeyEnvar: "AZURE_OPENAI_API_KEY",
			},
		},
		{
			name:         "No provider specified",
			input:        "gpt-4",
			wantProvider: "",
			wantModel:    "gpt-4",
			wantInfo:     providerInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProvider, gotModel, gotInfo := parseModel(tt.input)
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
