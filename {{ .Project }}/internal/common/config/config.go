package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const envPrefix = "SERVICE_"

type Override struct {
	Environment string
	Key         string
	Value       string
}

func LoadDir(dir string) (*Config, []Override, error) {
	values, overrides, err := LoadValues(dir)
	if err != nil {
		return nil, nil, err
	}
	var cfg Config
	if err := values.Unmarshal("", &cfg); err != nil {
		return nil, nil, fmt.Errorf("decode configuration: %w", err)
	}
	return &cfg, overrides, nil
}

// LoadValues shares the same YAML and indexed environment loading between the
// application and generators without requiring callers to decode every field.
func LoadValues(dir string) (*koanf.Koanf, []Override, error) {
	values := koanf.New(".")
	paths, err := yamlFiles(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, path := range paths {
		if err := values.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, nil, fmt.Errorf("load config file %q: %w", path, err)
		}
	}
	overrides, err := applyEnvironment(values)
	if err != nil {
		return nil, nil, fmt.Errorf("load environment config: %w", err)
	}
	return values, overrides, nil
}

func LogOverrides(overrides []Override) {
	for _, override := range overrides {
		slog.Info("load env: " + override.Environment + "=" + override.Value)
	}
}

func yamlFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read config directory %q: %w", dir, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yml" || ext == ".yaml" {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no yaml configuration files found in %q", dir)
	}
	return paths, nil
}
