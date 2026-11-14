package main

import (
	"errors"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const goPluginVersion = "v1.36.12"

const grpcPluginVersion = "v1.6.2"

func protoFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == dir {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".proto") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func ensurePlugin(dir, name, module, version string) (string, error) {
	executable := filepath.Join(dir, name)
	if output, err := exec.Command(executable, "--version").Output(); err == nil && strings.HasSuffix(strings.TrimSpace(string(output)), strings.TrimPrefix(version, "v")) {
		return executable, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	fmt.Printf("install %s %s\n", name, version)
	command := exec.Command("go", "install", module+"@"+version)
	command.Env = append(os.Environ(), "GOBIN="+dir)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("install %s: %w", name, err)
	}
	return executable, nil
}

// Map Go imports locally while preserving the provider's Proto contract.
func serviceMapping(source, modulePath string) (string, error) {
	parts := strings.Split(source, "/")
	if len(parts) < 3 || parts[0] != "api" || !strings.HasSuffix(parts[1], "-proto") {
		return "", fmt.Errorf("proto sources must be under api/<service>-proto: %s", source)
	}
	service := strings.TrimSuffix(parts[1], "-proto")
	alias := strings.ReplaceAll(service, "-", "_")
	if !token.IsIdentifier(alias) {
		return "", fmt.Errorf("invalid proto service directory: %s", parts[1])
	}
	return source + "=" + modulePath + "/api/" + service + ";" + alias, nil
}
