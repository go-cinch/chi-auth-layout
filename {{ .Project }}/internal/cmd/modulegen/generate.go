package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/genfile"
)

func run(modulesDir, appFile, goMod, output string) error {
	modulePath, err := readModulePath(goMod)
	if err != nil {
		return err
	}
	fields, err := applicationFields(appFile)
	if err != nil {
		return err
	}
	modulesRelative, err := filepath.Rel(filepath.Dir(goMod), modulesDir)
	if err != nil {
		return fmt.Errorf("resolve modules directory: %w", err)
	}
	modulesImport := modulePath + "/" + filepath.ToSlash(modulesRelative)
	specs, err := discoverModules(modulesDir, modulesImport, fields)
	if err != nil {
		return err
	}
	source, err := generate(modulePath, specs)
	if err != nil {
		return err
	}
	return genfile.Write(output, source)
}

func readModulePath(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read go module: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", errors.New("go module path not found")
}
