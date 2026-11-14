package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	configDir := flag.String("config-dir", "conf", "configuration directory")
	output := flag.String("output", "internal/common/config/config.gen.go", "generated Go file")
	flag.Parse()
	if err := run(*configDir, *output); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "configgen: %v\n", err)
		os.Exit(1)
	}
}
