package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/sashabaranov/go-openai"
)

func convertFunctionsToTools(functions []FunctionConfig) []openai.Tool {
	var tools []openai.Tool
	for _, fc := range functions {
		function := convertToOpenAIFunction(fc)
		tools = append(tools, openai.Tool{
			Type:     openai.ToolTypeFunction,
			Function: &function,
		})
	}
	return tools
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

func executeFunction(askLevel string, fc FunctionConfig, args string, showCommands bool) (string, string, error) {
	parsedArgs, err := parseAndValidateArgs(fc, args)
	if err != nil {
		return "", "", err
	}

	command, err := prepareCommand(fc, parsedArgs)
	if err != nil {
		return "", "", err
	}

	origCommand := command
	command = expandHomePath(command)

	// Check if confirmation is needed
	if needsConfirmation(askLevel, fc.Safe) {
		if !confirm(fmt.Sprintf("Execute '%s'?", command)) {
			return command, "Command execution cancelled by user.", nil
		}
	}

	if showCommands {
		fmt.Fprintf(os.Stderr, "Command: %s\n", command)
	}

	output, err := executeShellCommand(command, fc, parsedArgs)
	if err != nil {
		return command, "", err
	}

	return origCommand, strings.TrimSpace(string(output)), nil
}

func parseAndValidateArgs(fc FunctionConfig, args string) (map[string]interface{}, error) {
	if args == "" {
		return make(map[string]interface{}), nil
	}

	var parsedArgs map[string]interface{}
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return nil, fmt.Errorf("error parsing arguments: %v", err)
	}

	// Validate required parameters
	var missingParams []string
	for _, param := range fc.Parameters {
		if param.Required {
			if value, exists := parsedArgs[param.Name]; !exists || value == nil {
				missingParams = append(missingParams, param.Name)
			}
		}
	}

	if len(missingParams) > 0 {
		return nil, fmt.Errorf("missing required parameters: %s", strings.Join(missingParams, ", "))
	}

	return parsedArgs, nil
}

func prepareCommand(fc FunctionConfig, parsedArgs map[string]interface{}) (string, error) {
	command := fc.Command

	// First, process any shell command blocks in the command
	var err error
	command, err = processShellBlocks(command)
	if err != nil {
		return "", fmt.Errorf("error processing shell blocks in command: %v", err)
	}

	// Replace parameters with their values
	for _, param := range fc.Parameters {
		placeholder := fmt.Sprintf("{{%s}}", param.Name)

		if value, exists := parsedArgs[param.Name]; exists {
			replacement, err := getParameterReplacement(param, value)
			if err != nil {
				return "", err
			}
			command = strings.ReplaceAll(command, placeholder, replacement)
		} else if !param.Required {
			command = strings.ReplaceAll(command, placeholder, "")
		}
	}

	// Clean up any extra spaces from removed optional parameters
	return strings.Join(strings.Fields(command), " "), nil
}

// processShellBlocks processes {{$...}} blocks in a string by executing them as shell commands
// and replacing them with the command output
func processShellBlocks(input string) (string, error) {
	blocksRegex := regexp.MustCompile(`{{\$(.*?)}}`)
	result := blocksRegex.ReplaceAllStringFunc(input, func(match string) string {
		command := match[3 : len(match)-2] // Extract command without {{$ and }}
		cmd := exec.Command("sh", "-c", command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return strings.TrimSpace(string(output))
	})
	return result, nil
}

func getParameterReplacement(param ParameterConfig, value interface{}) (string, error) {
	switch {
	case param.Format == "boolean":
		boolValue, err := strconv.ParseBool(fmt.Sprintf("%v", value))
		if err != nil {
			return "", fmt.Errorf("invalid boolean value: %v", value)
		}
		if boolValue {
			return param.Format, nil
		}
		return "", nil

	case param.Format != "" && !strings.Contains(param.Format, "%"):
		return param.Format, nil

	case param.Format != "":
		return fmt.Sprintf(param.Format, value), nil

	default:
		return fmt.Sprintf("%v", value), nil
	}
}

func needsConfirmation(askLevel string, isSafe bool) bool {
	if askLevel == "" {
		askLevel = "unsafe"
	}
	return askLevel == "all" || (askLevel == "unsafe" && !isSafe)
}

func executeShellCommand(command string, fc FunctionConfig, args map[string]interface{}) ([]byte, error) {
	if fc.Output != "" {
		// Process output template similar to command
		formattedOutput, err := processShellBlocks(fc.Output)
		if err != nil {
			return nil, fmt.Errorf("error processing output template: %v", err)
		}

		// Replace parameters in output template
		for _, param := range fc.Parameters {
			placeholder := fmt.Sprintf("{{%s}}", param.Name)
			if value, exists := args[param.Name]; exists {
				replacement, err := getParameterReplacement(param, value)
				if err != nil {
					return nil, err
				}
				formattedOutput = strings.ReplaceAll(formattedOutput, placeholder, replacement)
			}
		}

		fmt.Print(formattedOutput)
	}

	cmd := exec.Command("sh", "-c", command)

	if fc.Stdin != "" {
		stdinContent := prepareStdinContent(fc.Stdin, args)
		cmd.Stdin = strings.NewReader(stdinContent)
	} else {
		cmd.Stdin = os.Stdin
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v\nCommand: %s\nOutput: %s", err, command, output)
	}

	return output, nil
}

func prepareStdinContent(stdinTemplate string, args map[string]interface{}) string {
	// First, process any shell command blocks
	processed, err := processShellBlocks(stdinTemplate)
	if err != nil {
		// If there's an error, just continue with the original template
		processed = stdinTemplate
	}

	// Then replace parameter placeholders
	for key, value := range args {
		placeholder := fmt.Sprintf("{{%s}}", key)
		processed = strings.ReplaceAll(processed, placeholder, fmt.Sprintf("%v", value))
	}
	return processed
}
