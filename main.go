package main

import (
	"fmt"
	"log"
	"os"
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
			if opts.AgentPath == DefaultAgentPath {
				fmt.Printf(`Default agent not found at %s

To get started:
1. Create an agent directory: mkdir -p ~/.config/esa/agents
2. Create a default agent config:
   See example configurations in the GitHub repo under /examples
   https://github.com/meain/esa/tree/master/examples
3. Optionally create additional configs for different agents
`, DefaultAgentPath)
				os.Exit(1)
			}

			log.Fatalf("Failed to initialize application: %v", err)
		}

		app.Run(opts)
	}
}
