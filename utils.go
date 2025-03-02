package main

import (
	"fmt"
	"strings"
)

// confirm prompts the user for confirmation with a yes/no question
func confirm(prompt string) bool {
	var response string
	fmt.Printf("%s (y/n): ", prompt)
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y"
}
