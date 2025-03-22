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
		handleShowAgent(opts.ConfigPath)
	case ListAgents:
		listAgents()
	case NormalExecution:
		app, err := NewApplication(&opts)
		if err != nil {
			if opts.ConfigPath == DefaultConfigPath {
				fmt.Printf(`Default config not found at %s

To get started:
1. Create a config directory: mkdir -p ~/.config/esa
2. Create a default config:
   See example configurations in the GitHub repo under /examples
   https://github.com/meain/esa/tree/master/examples
3. Optionally create additional configs for different agents
`, DefaultConfigPath)
				os.Exit(1)
			}

			log.Fatalf("Failed to initialize application: %v", err)
		}

		app.Run(opts)
	}
}
