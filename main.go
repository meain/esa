package main

import (
	"log"
)

func main() {
	opts, commandType := parseFlags()

	switch commandType {
	case ShowAgent:
		handleShowAgent(opts.AgentPath)
	case ListAgents:
		listAgents()
	case ListHistory:
		listHistory()
	case ShowHistory:
		handleShowHistory(opts.HistoryIndex, opts.OutputFormat)
	case NormalExecution:
		app, err := NewApplication(&opts)
		if err != nil {
			log.Fatalf("Failed to initialize application: %v", err)
		}

		app.Run(opts)
	}
}
