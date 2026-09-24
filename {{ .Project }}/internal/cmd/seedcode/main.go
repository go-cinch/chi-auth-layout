package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/code"
)

const (
	defaultSeedFile       = "internal/infra/db/migrations/YYYYMMDDHH-02-auth-default-data.sql"
	maxGenerationAttempts = 1024
)

var (
	seedCodePattern      = regexp.MustCompile(`\b[A-Z0-9]{8}\b`)
	generatedCodePattern = regexp.MustCompile(`^[23456789ABCDEFGHJKLMNPQRSTVWXY]{8}$`)
)

func main() {
	path := flag.String("file", defaultSeedFile, "default-data migration to rewrite")
	flag.Parse()

	count, err := rewriteSeedCodes(*path, code.Generate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "randomize default seed codes: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Randomized %d default seed codes.\n", count)
}

func rewriteSeedCodes(path string, generate func() (string, error)) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}

	placeholders := uniqueCodes(seedCodePattern.FindAllString(string(content), -1))
	if len(placeholders) == 0 {
		return 0, fmt.Errorf("no eight-character seed codes found in %s", path)
	}

	used := make(map[string]struct{}, len(placeholders)*2)
	for _, placeholder := range placeholders {
		used[placeholder] = struct{}{}
	}

	replacements := make([]string, 0, len(placeholders)*2)
	for _, placeholder := range placeholders {
		generated, err := nextUniqueCode(generate, used)
		if err != nil {
			return 0, fmt.Errorf("replace seed code %s: %w", placeholder, err)
		}
		used[generated] = struct{}{}
		replacements = append(replacements, placeholder, generated)
	}

	rewritten := strings.NewReplacer(replacements...).Replace(string(content))
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(rewritten), info.Mode().Perm()); err != nil {
		return 0, fmt.Errorf("write %s: %w", path, err)
	}
	return len(placeholders), nil
}

func uniqueCodes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func nextUniqueCode(generate func() (string, error), used map[string]struct{}) (string, error) {
	for range maxGenerationAttempts {
		value, err := generate()
		if err != nil {
			return "", err
		}
		if !generatedCodePattern.MatchString(value) {
			return "", fmt.Errorf("generator returned invalid code %q", value)
		}
		if _, exists := used[value]; !exists {
			return value, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique code after %d attempts", maxGenerationAttempts)
}
