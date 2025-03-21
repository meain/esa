package main

import (
	"log"
)

func main() {
	opts := parseFlags()

	app, err := NewApplication(opts)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	app.Run(opts)
}
