package main

import (
	"flag"
	"fmt"
	"os"

	"{{ .Computed.module_name_final }}/internal/common/code"
)

func main() {
	var paths []string
	flag.Func("file", "default-data migration to rewrite; may be repeated", func(path string) error {
		paths = append(paths, path)
		return nil
	})
	flag.Parse()
	if len(paths) == 0 {
		paths = []string{defaultSeedFile, defaultActionSeedFile}
	}

	count, err := rewriteSeedFiles(paths, code.Generate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "randomize default seed codes: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Randomized %d default seed codes.\n", count)
}
