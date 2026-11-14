package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	modulesDir := flag.String("modules-dir", "internal/modules", "business modules directory")
	appFile := flag.String("app-file", "internal/app/app.go", "file containing Application")
	goMod := flag.String("go-mod", "go.mod", "Go module file")
	output := flag.String("output", "internal/app/modules.gen.go", "generated Go file")
	flag.Parse()
	if err := run(*modulesDir, *appFile, *goMod, *output); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "modulegen: %v\n", err)
		os.Exit(1)
	}
}
