package main

import (
	"embed"
	"io/fs"
)

//go:embed web/*
var webContent embed.FS

// webFS provides the web directory as the root
var webFS, _ = fs.Sub(webContent, "web")
