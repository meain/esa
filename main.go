package main

import (
	"log"
)

func main() {
	opts, commandType := parseFlags()

	switch commandType {
	case ShowAgent:
		handleShowAgent(opts.ConfigPath)
	case ListAgents:
		listAgents()
	case NormalExecution:
		app, err := NewApplication(opts)
		if err != nil {
			log.Fatalf("Failed to initialize application: %v", err)
		}

		app.Run(opts)
	}
}
