package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sashabaranov/go-openai"
)

type FunctionConfig struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description"`
	Command     string            `toml:"command"`
	Parameters  []ParameterConfig `toml:"parameters"`
}

type ParameterConfig struct {
	Name        string `toml:"name"`
	Type        string `toml:"type"`
	Description string `toml:"description"`
	Required    bool   `toml:"required"`
}

type Config struct {
	Functions []FunctionConfig `toml:"functions"`
}

func expandHomePath(path string) string {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

func loadConfig(configPath string) (Config, error) {
	var config Config
	_, err := toml.DecodeFile(expandHomePath(configPath), &config)
	return config, err
}

func convertToOpenAIFunction(fc FunctionConfig) openai.FunctionDefinition {
	properties := make(map[string]interface{})
	required := []string{}

	for _, param := range fc.Parameters {
		properties[param.Name] = map[string]string{
			"type":        param.Type,
			"description": param.Description,
		}
		if param.Required {
			required = append(required, param.Name)
		}
	}

	return openai.FunctionDefinition{
		Name:        fc.Name,
		Description: fc.Description,
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": properties,
			"required":   required,
		},
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: esa <command>")
		os.Exit(1)
	}

	// Load configuration
	configPath := "~/.config/esa/config.toml"
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	fullCommand := os.Args[1:]
	commandStr := String(fullCommand)

	// Initialize OpenAI client
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// Convert function configs to OpenAI function definitions
	var openAIFunctions []openai.FunctionDefinition
	for _, fc := range config.Functions {
		openAIFunctions = append(openAIFunctions, convertToOpenAIFunction(fc))
	}

	// Create chat completion request with function calling
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Content: commandStr,
				},
			},
			Functions: openAIFunctions,
		})

	if err != nil {
		log.Fatalf("Chat completion error: %v", err)
	}

	// Check if a function call was suggested
	if resp.Choices[0].Message.FunctionCall != nil {
		functionCall := resp.Choices[0].Message.FunctionCall

		// Find the corresponding function config
		var matchedFunc FunctionConfig
		for _, fc := range config.Functions {
			if fc.Name == functionCall.Name {
				matchedFunc = fc
				break
			}
		}

		if matchedFunc.Name == "" {
			log.Fatalf("No matching function found for: %s", functionCall.Name)
		}

		// Execute the function
		result, err := executeFunction(matchedFunc, functionCall.Arguments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Action completed:", result)
	} else {
		fmt.Println("No specific action could be determined.")
	}
}

func executeFunction(fc FunctionConfig, args string) (string, error) {
	// Parse the JSON arguments
	var parsedArgs map[string]interface{}
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("error parsing arguments: %v", err)
	}

	// Prepare the command by replacing placeholders
	command := fc.Command
	for key, value := range parsedArgs {
		placeholder := fmt.Sprintf("{{%s}}", key)
		command = strings.ReplaceAll(command, placeholder, fmt.Sprintf("%v", value))
	}

	// Expand any home path in the command
	command = expandHomePath(command)

	// Execute the command
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v\nCommand: %s\nOutput: %s", err, command, output)
	}

	return strings.TrimSpace(string(output)), nil
}

func String(args []string) string {
	return strings.Join(args, " ")
}
