package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func createNewHistoryFile(cacheDir string, agentName string) string {
	if agentName == "" {
		agentName = "default"
	}
	timestamp := time.Now().Format(historyTimeFormat)
	return filepath.Join(cacheDir, fmt.Sprintf("%s-%s.json", agentName, timestamp))
}

func findLatestHistoryFile(cacheDir string) (string, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return "", err
	}

	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if latestFile == "" || info.ModTime().After(latestTime) {
				latestFile = entry.Name()
				latestTime = info.ModTime()
			}
		}
	}

	if latestFile == "" {
		return "", fmt.Errorf("no history files found")
	}
	return filepath.Join(cacheDir, latestFile), nil
}

func getHistoryFilePath(cacheDir string, opts *CLIOptions) (string, bool) {
	if !opts.ContinueChat && !opts.RetryChat {
		cacheDir, err := setupCacheDir()
		if err != nil {
			// Handle error appropriately, maybe log or return an error
			// For now, fallback or panic might occur depending on createNewHistoryFile
			fmt.Fprintf(os.Stderr, "Warning: could not get cache dir: %v\n", err)
		}
		return createNewHistoryFile(cacheDir, opts.AgentName), false
	}

	if latestFile, err := findLatestHistoryFile(cacheDir); err == nil {
		return latestFile, true
	}

	cacheDir, err := setupCacheDir()
	if err != nil {
		// Handle error appropriately
		fmt.Fprintf(os.Stderr, "Warning: could not get cache dir: %v\n", err)
	}
	return createNewHistoryFile(cacheDir, opts.AgentName), false
}

// Read stdin if exists. Used to detect if input is piped and read it if so
func readStdin() string {
	var input bytes.Buffer
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		if _, err := io.Copy(&input, os.Stdin); err != nil {
			return ""
		}
	}
	return input.String()
}

func readUserInput(prompt string, multiline bool) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	var lines []string

	if prompt != "" {
		color.New(color.FgBlue).Fprint(os.Stderr, prompt)
		color.New(color.FgHiWhite, color.Italic).Fprint(os.Stderr, " (end with empty line)\n")
	}

	// TODO(meain): allow for newline using shift+enter
	// Not sure if that will be something that terminal supports, but
	// we might be able to do that with some closing char and possibly
	// with ability to jump into a text editor.
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		line = strings.TrimRight(line, "\r\n")
		if !multiline {
			return line, nil
		}

		if line == "" {
			break
		}
		lines = append(lines, line)
	}

	result := strings.Join(lines, "\n")
	return result, nil
}
