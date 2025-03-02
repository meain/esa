package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type CLIOptions struct {
	DebugMode    bool
	ContinueChat bool
	ConfigPath   string
	AskLevel     string
	ShowCommands bool
	CommandStr   string
	AgentName    string
}

func parseFlags() CLIOptions {
	opts := CLIOptions{}

	flag.BoolVar(&opts.DebugMode, "debug", false, "Enable debug mode")
	flag.BoolVar(&opts.ContinueChat, "c", false, "Continue last conversation")
	flag.BoolVar(&opts.ContinueChat, "continue", false, "Continue last conversation")
	configPath := flag.String("config", "~/.config/esa/config.toml", "Path to the config file")
	flag.StringVar(&opts.AskLevel, "ask", "none", "Ask level (none, unsafe, all)")
	flag.BoolVar(&opts.ShowCommands, "show-commands", false, "Show executed commands")
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	args := flag.Args()
	opts.CommandStr = strings.Join(args, " ")
	opts.ConfigPath = *configPath

	if strings.HasPrefix(opts.CommandStr, "+") {
		parts := strings.SplitN(opts.CommandStr, " ", 2)
		if len(parts) < 2 {
			fmt.Println("Usage: esa +<config> <command>")
			os.Exit(1)
		}
		opts.AgentName = parts[0][1:]
		opts.CommandStr = parts[1]
		opts.ConfigPath = fmt.Sprintf("~/.config/esa/%s.toml", opts.AgentName)
	}

	return opts
}

func printHelp() {
	fmt.Println("Usage: esa <command> [--debug] [--config <path>] [--ask <level>]")
	fmt.Println("\nCommands:")
	fmt.Println("  list-functions    List all available functions")
	fmt.Println("  <text>           Send text command to the assistant")
}

func printFunctionInfo(fn FunctionConfig) {
	fmt.Printf("%s\n", fn.Name)
	fmt.Printf("  %s\n", fn.Description)
	if len(fn.Parameters) > 0 {
		for _, p := range fn.Parameters {
			required := ""
			if p.Required {
				required = " (required)"
			}
			fmt.Printf("  • %s: %s%s\n", p.Name, p.Description, required)
		}
	}
	fmt.Println()
}
