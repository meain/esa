package main

import (
	"log"
)

func main() {
	opts := parseFlags()
	if opts.CommandStr == "list-functions" {
		handleListFunctions(opts.ConfigPath)
		return
	}

	app, err := NewApplication(opts)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	app.Run(opts)
}
