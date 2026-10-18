package main

import (
	"fmt"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/config"
)

// Configured modules declare their mount's YAML key alongside Name and HTTP.
// Resolve that scalar with the same environment overrides used by the application.
func configureModulePaths(packages []*sourcePackage, configDir string) error {
	values, _, err := config.LoadValues(configDir)
	if err != nil {
		return err
	}
	for _, pkg := range packages {
		for name, function := range pkg.funcs {
			if !strings.HasSuffix(name, ".HTTPPathConfig") {
				continue
			}
			key := returnedString(function)
			if key == "" {
				return fmt.Errorf("HTTPPathConfig must return a literal configuration key")
			}
			if raw := values.Get(key); raw != nil {
				if _, ok := raw.(string); !ok {
					return fmt.Errorf("%s must be a string", key)
				}
			}
			prefix, err := config.NormalizeHTTPPath(values.String(key))
			if err != nil {
				return err
			}
			pkg.receivers[strings.TrimSuffix(name, ".HTTPPathConfig")] = prefix
		}
	}
	return nil
}
