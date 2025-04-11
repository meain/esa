package main

import (
	_ "embed"
)

//go:embed builtins/new.toml
var newAgentToml string

//go:embed builtins/default.toml
var defaultAgentToml string
