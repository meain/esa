package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
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
	properties := make(map[string]any)
	required := []string{}

	for _, param := range fc.Parameters {
		paramProps := map[string]any{
			"type":        param.Type,
			"description": param.Description,
		}
		if len(param.Options) > 0 {
			paramProps["enum"] = param.Options
		}
		properties[param.Name] = paramProps
		if param.Required {
			required = append(required, param.Name)
		}
	}

	desc := fmt.Sprintf(
		"%s\n\nThe templated cli command that will be ran is: `%s`",
		fc.Description,
		fc.Command,
	)

	return openai.FunctionDefinition{
		Name:        fc.Name,
		Description: desc,
		Parameters: map[string]any{
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
		color.New(color.FgCyan).Fprintf(os.Stderr, "$ %s\n", command)
	}

	output, err := executeShellCommand(command, fc, parsedArgs)
	if err != nil {
		return command, "", err
	}

	return origCommand, strings.TrimSpace(string(output)), nil
}

func parseAndValidateArgs(fc FunctionConfig, args string) (map[string]any, error) {
	if args == "" {
		return make(map[string]any), nil
	}

	var parsedArgs map[string]any
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

func prepareCommand(fc FunctionConfig, parsedArgs map[string]any) (string, error) {
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

func readUserInput(prompt string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	var lines []string

	color.New(color.FgBlue).Fprint(os.Stderr, prompt)
	color.New(color.FgHiWhite, color.Italic).Fprint(os.Stderr, " (end with empty line)\n")

	// TODO(meain): allow for newline using shift+enter
	for {
		line, _ := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lines = append(lines, line)
	}

	result := strings.Join(lines, "\n")
	return result, nil
}

// processShellBlocks processes special blocks in a string:
// {{$...}} blocks are executed as shell commands and replaced with output
// {{#...}} blocks prompt for user input with the text as prompt
func processShellBlocks(input string) (string, error) {
	// Process shell command blocks {{$...}}
	shellRegex := regexp.MustCompile(`{{\$(.*?)}}`)
	result := shellRegex.ReplaceAllStringFunc(input, func(match string) string {
		command := match[3 : len(match)-2] // Extract command without {{$ and }}
		cmd := exec.Command("sh", "-c", command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return strings.TrimSpace(string(output))
	})

	// Process user input blocks {{#...}}
	inputRegex := regexp.MustCompile(`{{#(.*?)}}`)
	result = inputRegex.ReplaceAllStringFunc(result, func(match string) string {
		prompt := match[3 : len(match)-2] // Extract prompt without {{# and }}
		input, err := readUserInput(prompt)
		if err != nil {
			return fmt.Sprintf("Error: %v", err) // TODO(meain): not ideal
		}

		return input
	})

	return result, nil
}

func getParameterReplacement(param ParameterConfig, value any) (string, error) {
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

func executeShellCommand(command string, fc FunctionConfig, args map[string]any) ([]byte, error) {
	if fc.Output != "" {
		// Process output template similar to command
		formattedOutput, err := processShellBlocks(fc.Output)
		if err != nil {
			return nil, fmt.Errorf("error processing output template: %v", err)
		}

		// Replace parameters in output template
		// TODO(meain): we should do the replacement first then
		// process shell blocks so that I can template. More
		// importantly I can do `{{#{{question}}}}` and get the
		// answer
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

	// Use fc.Timeout instead of fc.TimeoutSec
	cmd := exec.Command("sh", "-c", command)

	// Set up context with timeout if specified
	ctx := context.Background()
	timeout := fc.Timeout
	if timeout <= 0 {
		timeout = 60 // default to 60 seconds if not set
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}

	// Set working directory if specified
	if fc.Pwd != "" {
		// Process templates in pwd similar to command
		pwd := fc.Pwd
		for _, param := range fc.Parameters {
			placeholder := fmt.Sprintf("{{%s}}", param.Name)
			if value, exists := args[param.Name]; exists {
				replacement, err := getParameterReplacement(param, value)
				if err != nil {
					return nil, err
				}
				pwd = strings.ReplaceAll(pwd, placeholder, replacement)
			}
		}
		pwd = expandHomePath(pwd)
		cmd.Dir = os.ExpandEnv(pwd) // Support environment variables in pwd
	}

	if fc.Stdin != "" {
		stdinContent := prepareStdinContent(fc.Stdin, args)
		cmd.Stdin = strings.NewReader(stdinContent)
	} else {
		cmd.Stdin = os.Stdin
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %v", err)
	}

	// Create a channel to wait for command completion
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Force kill the process if timeout is reached
		if err := cmd.Process.Kill(); err != nil {
			return nil, fmt.Errorf("failed to kill process: %v", err)
		}
		return nil, fmt.Errorf("command timed out and was killed: %s", command)
	case err := <-done:
		if err != nil {
			output, _ := cmd.CombinedOutput()
			return nil, fmt.Errorf("%v\nCommand: %s\nOutput: %s", err, command, output)
		}
		return cmd.CombinedOutput()
	}
}

func prepareStdinContent(stdinTemplate string, args map[string]any) string {
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
