package app

import (
	"os"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/config"
)

func e2eTestEnabledFromConfig(cfg *config.Config) func() bool {
	enabled := cfg.Auth.Switches.EnableE2ETest
	path := cfg.Auth.Switches.EnableE2ETestFile
	return func() bool {
		if path == "" {
			return enabled
		}
		// Reopen the path on every check: projected volumes replace symlink targets.
		// A configured file never falls back to the static flag on read failure.
		data, err := os.ReadFile(path)
		return err == nil && strings.TrimSpace(string(data)) == "true"
	}
}
