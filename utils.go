package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

// confirm prompts the user for confirmation with a yes/no question
func confirm(prompt string) bool {
	var response string
	cyan := color.New(color.FgCyan).SprintFunc()
	fmt.Fprintf(os.Stderr, "%s %s (y/N): ", cyan("[?]"), prompt)
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y"
}

// setupCacheDir ensures the cache directory exists and returns its path.
func setupCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	esaDir := filepath.Join(cacheDir, "esa")
	return esaDir, os.MkdirAll(esaDir, 0755)
}
