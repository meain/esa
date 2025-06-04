package main

import (
	_ "embed"
)

//go:embed builtins/new.toml
var newAgentToml string

//go:embed builtins/auto.toml
var autoAgentToml string

//go:embed builtins/default.toml
var defaultAgentToml string

var builtinAgents = map[string]string{
	"new":     newAgentToml,
	"auto":    autoAgentToml,
	"default": defaultAgentToml,
}
