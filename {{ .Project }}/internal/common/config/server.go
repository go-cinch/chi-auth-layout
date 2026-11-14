package config

import (
	"fmt"
	"regexp"
)

var namespaceSegment = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateServer rejects ambiguous namespace segments before infrastructure starts.
func (c *Config) ValidateServer() error {
	for _, field := range []struct{ name, value string }{
		{"env", c.Server.Env}, {"name", c.Server.Name},
	} {
		if !namespaceSegment.MatchString(field.value) {
			return fmt.Errorf("server.%s must be a non-empty namespace segment (letters, digits, hyphens, underscores or dots)", field.name)
		}
	}
	return nil
}

// RedisPrefix is derived after YAML and environment overrides have been loaded.
func (c *Config) RedisPrefix() string { return c.Server.Env + ":" + c.Server.Name + ":" }
