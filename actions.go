package main

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/sashabaranov/go-openai"
)

// SetAlarmFunction defines the function for setting an alarm
func SetAlarmFunction() openai.FunctionDefinition {
	return openai.FunctionDefinition{
		Name: "set_alarm",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"time": {
					"type": "string",
					"description": "Time to set the alarm in HH:mm format"
				},
				"label": {
					"type": "string",
					"description": "Optional label for the alarm"
				}
			},
			"required": ["time"]
		}`),
	}
}

// OpenWebsiteFunction defines the function for opening a website
func OpenWebsiteFunction() openai.FunctionDefinition {
	return openai.FunctionDefinition{
		Name: "open_website",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"url": {
					"type": "string",
					"description": "Website URL to open"
				}
			},
			"required": ["url"]
		}`),
	}
}

// AdjustBrightnessFunction defines the function for adjusting screen brightness
func AdjustBrightnessFunction() openai.FunctionDefinition {
	return openai.FunctionDefinition{
		Name: "adjust_brightness",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"level": {
					"type": "string",
					"enum": ["increase", "decrease"],
					"description": "Direction to adjust brightness"
				}
			},
			"required": ["level"]
		}`),
	}
}

// SendMessageFunction defines the function for sending a message
func SendMessageFunction() openai.FunctionDefinition {
	return openai.FunctionDefinition{
		Name: "send_message",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"recipient": {
					"type": "string",
					"description": "Name or contact of the recipient"
				},
				"message": {
					"type": "string",
					"description": "Content of the message"
				}
			},
			"required": ["recipient", "message"]
		}`),
	}
}

// ExecuteFunctionCall handles the execution of function calls
func ExecuteFunctionCall(functionCall *openai.FunctionCall) (string, error) {
	switch functionCall.Name {
	case "set_alarm":
		return executeSetAlarm(functionCall.Arguments)
	case "open_website":
		return executeOpenWebsite(functionCall.Arguments)
	case "adjust_brightness":
		return executeAdjustBrightness(functionCall.Arguments)
	case "send_message":
		return executeSendMessage(functionCall.Arguments)
	default:
		return "", fmt.Errorf("unknown function: %s", functionCall.Name)
	}
}

// executeSetAlarm sets an alarm using AppleScript
func executeSetAlarm(args string) (string, error) {
	var params struct {
		Time  string `json:"time"`
		Label string `json:"label,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	script := fmt.Sprintf(`
		tell application "System Events"
			activate
			set alarmTime to time "%s"
			make new alarm with properties {time:alarmTime}
		end tell
	`, params.Time)

	err := exec.Command("osascript", "-e", script).Run()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Alarm set for %s", params.Time), nil
}

// executeOpenWebsite opens a website
func executeOpenWebsite(args string) (string, error) {
	var params struct {
		URL string `json:"url"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	err := exec.Command("open", params.URL).Run()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Opened %s", params.URL), nil
}

// executeAdjustBrightness adjusts screen brightness
func executeAdjustBrightness(args string) (string, error) {
	var params struct {
		Level string `json:"level"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	var script string
	if params.Level == "decrease" {
		script = `
			tell application "System Events"
				key code 107  # Brightness down
			end tell
		`
	} else {
		script = `
			tell application "System Events"
				key code 113  # Brightness up
			end tell
		`
	}

	err := exec.Command("osascript", "-e", script).Run()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Brightness %sed", params.Level), nil
}

// executeSendMessage sends a message (placeholder implementation)
func executeSendMessage(args string) (string, error) {
	var params struct {
		Recipient string `json:"recipient"`
		Message   string `json:"message"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	// Placeholder for message sending logic
	// In a real implementation, this could integrate with iMessage, Slack, etc.
	return fmt.Sprintf("Sent message to %s", params.Recipient), nil
}
