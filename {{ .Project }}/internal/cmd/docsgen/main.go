package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	source := flag.String("source", "internal", "Go source directory")
	configDir := flag.String("config-dir", "conf", "configuration directory")
	output := flag.String("output", "internal/docs/openapi.yaml", "generated OpenAPI file")
	title := flag.String("title", "HTTP API", "OpenAPI title")
	flag.Parse()
	if err := run(*source, *configDir, *output, *title); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "docsgen: %v\n", err)
		os.Exit(1)
	}
}
