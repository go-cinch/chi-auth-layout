package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func run(root string) error {
	inputs, err := protoFiles(filepath.Join(root, "api"))
	if err != nil {
		return err
	}
	output, err := os.MkdirTemp("", "proto-output-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(output)
	if len(inputs) > 0 {
		modulePath, err := readModulePath(filepath.Join(root, "go.mod"))
		if err != nil {
			return err
		}
		compiler, err := exec.LookPath("protoc")
		if err != nil {
			return errors.New("protoc is required for api/*.proto; install protobuf (brew install protobuf or apt install protobuf-compiler)")
		}
		toolsDir, err := filepath.Abs(filepath.Join(root, ".tools"))
		if err != nil {
			return err
		}
		goPlugin, err := ensurePlugin(toolsDir, "protoc-gen-go", "google.golang.org/protobuf/cmd/protoc-gen-go", goPluginVersion)
		if err != nil {
			return err
		}
		grpcPlugin, err := ensurePlugin(toolsDir, "protoc-gen-go-grpc", "google.golang.org/grpc/cmd/protoc-gen-go-grpc", grpcPluginVersion)
		if err != nil {
			return err
		}
		args := []string{"-I.", "--plugin=protoc-gen-go=" + goPlugin, "--plugin=protoc-gen-go-grpc=" + grpcPlugin, "--go_out=" + output, "--go_opt=module=" + modulePath, "--go-grpc_out=" + output, "--go-grpc_opt=module=" + modulePath}
		if info, err := os.Stat(filepath.Join(root, "third_party")); err == nil && info.IsDir() {
			args = append(args, "-Ithird_party")
		}
		for _, input := range inputs {
			relative, err := filepath.Rel(root, input)
			if err != nil {
				return err
			}
			source := filepath.ToSlash(relative)
			mapping, err := serviceMapping(source, modulePath)
			if err != nil {
				return err
			}
			args = append(args, "--go_opt=M"+mapping, "--go-grpc_opt=M"+mapping, source)
		}
		command := exec.Command(compiler, args...)
		command.Dir = root
		if data, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("compile proto: %w: %s", err, strings.TrimSpace(string(data)))
		}
	}
	return syncGenerated(root, output)
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
