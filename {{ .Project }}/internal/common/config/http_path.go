package config

import (
	"fmt"
	"regexp"
)

var httpPathPattern = regexp.MustCompile(`^(/[A-Za-z0-9_-]+)*$`)

// NormalizeHTTPPath accepts an empty root mount or an absolute module prefix.
func NormalizeHTTPPath(value string) (string, error) {
	if value == "/" {
		return "", nil
	}
	if len(value) > 256 || !httpPathPattern.MatchString(value) {
		return "", fmt.Errorf("invalid auth.path %q: use an empty value or an absolute path without a trailing slash", value)
	}
	return value, nil
}
