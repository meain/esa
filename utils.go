package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
)

// confirmResponse represents the response type from a confirmation prompt
type confirmResponse struct {
	approved bool
	message  string
}

// confirm prompts the user for confirmation with yes/no/message options
func confirm(prompt string) confirmResponse {
	var response string
	cyan := color.New(color.FgCyan).SprintFunc()
	fmt.Fprintf(os.Stderr, "%s %s (m/y/N): ", cyan("[?]"), prompt)
	fmt.Scanln(&response)
	response = strings.ToLower(response)

	if response == "m" {
		fmt.Fprintf(os.Stderr, "%s Enter message: ", cyan("[?]"))
		message, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		message = strings.TrimSuffix(message, "\n")
		return confirmResponse{approved: false, message: message}
	}

	return confirmResponse{approved: response == "y", message: ""}
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

func findHistoryFile(cacheDir string, index int) (string, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return "", err
	}

	type fileEntry struct {
		name    string
		modTime time.Time
	}

	var files []fileEntry

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, fileEntry{
				name:    entry.Name(),
				modTime: info.ModTime(),
			})
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no history files found")
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	if index < 0 || index >= len(files) {
		return "", fmt.Errorf("history file index %d out of range (0-%d)", index, len(files)-1)
	}

	return filepath.Join(cacheDir, files[index].name), nil
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

	if latestFile, err := findHistoryFile(cacheDir, opts.ConversationIndex-1); err == nil {
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
	if prompt != "" {
		color.New(color.FgBlue).Fprint(os.Stderr, prompt)
		color.New(color.FgHiWhite, color.Italic).Fprint(os.Stderr, " (ctrl+d on empty line to complete)\n")
	}

	var result strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				// Ctrl+D pressed
				break
			}
			return "", err
		}

		// Return if we just want a single line
		if !multiline {
			return line, nil
		}

		// Remove the trailing newline and add to result
		line = strings.TrimSuffix(line, "\n")

		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		result.WriteString(line)

		// Check if line is empty and we got EOF (Ctrl+D)
		if line == "" {
			nextByte, err := reader.ReadByte()
			if err != nil && err.Error() == "EOF" {
				break
			}
			if err == nil {
				// Put the byte back by creating a new reader with it
				result.WriteByte('\n')
				remaining, _ := reader.ReadString('\n')
				result.WriteString(string(nextByte) + remaining)
			}
		}
	}

	return result.String(), nil
}
