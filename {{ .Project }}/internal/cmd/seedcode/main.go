package main

import (
	"flag"
	"fmt"
	"github.com/pmezard/go-difflib/difflib"
	"os"
	"regexp"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/code"
)

const (
	defaultSeedFile       = "internal/infra/db/migrations/YYYYMMDDHH-02-auth-default-data.sql"
	defaultActionSeedFile = "internal/infra/db/migrations/YYYYMMDDHH-03-action-default-data.sql"
	maxGenerationAttempts = 1024
)

var (
	sqlStringPattern     = regexp.MustCompile(`'(?:[^']|'')*'`)
	seedCodePattern      = regexp.MustCompile(`^[A-Z0-9]{8}(?:,[A-Z0-9]{8})*$`)
	generatedCodePattern = regexp.MustCompile(`^[23456789ABCDEFGHJKLMNPQRSTVWXY]{8}$`)
)

func main() {
	var paths []string
	flag.Func("file", "default-data migration to rewrite; may be repeated", func(path string) error {
		paths = append(paths, path)
		return nil
	})
	flag.Parse()
	if len(paths) == 0 {
		paths = []string{defaultSeedFile, defaultActionSeedFile}
	}

	count, err := rewriteSeedFiles(paths, code.Generate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "randomize default seed codes: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Randomized %d default seed codes.\n", count)
}

func rewriteSeedCodes(path string, generate func() (string, error)) (int, error) {
	return rewriteSeedFiles([]string{path}, generate)
}

func rewriteSeedFiles(paths []string, generate func() (string, error)) (int, error) {
	type seedFile struct {
		path    string
		content []byte
		mode    os.FileMode
	}
	files := make([]seedFile, 0, len(paths))
	var placeholders []string
	// Read every file before generating or writing any replacement. One mapping
	// keeps action definitions and assignments consistent across split migrations.
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return 0, fmt.Errorf("read %s: %w", path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return 0, fmt.Errorf("stat %s: %w", path, err)
		}
		files = append(files, seedFile{path: path, content: content, mode: info.Mode().Perm()})
		// Match only whole SQL string values, not SQL keywords or JSON contents.
		for _, literal := range sqlStringPattern.FindAllString(string(content), -1) {
			value := literal[1 : len(literal)-1]
			if seedCodePattern.MatchString(value) {
				placeholders = append(placeholders, strings.Split(value, ",")...)
			}
		}
	}
	placeholders = uniqueCodes(placeholders)
	if len(placeholders) == 0 {
		return 0, fmt.Errorf("no eight-character seed codes found in %v", paths)
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

	replacer := strings.NewReplacer(replacements...)
	for _, file := range files {
		rewritten := sqlStringPattern.ReplaceAllStringFunc(string(file.content), func(literal string) string {
			if !seedCodePattern.MatchString(literal[1 : len(literal)-1]) {
				return literal
			}
			return replacer.Replace(literal)
		})
		if rewritten == string(file.content) {
			fmt.Printf("%s unchanged\n", file.path)
			continue
		}
		diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: difflib.SplitLines(string(file.content)), B: difflib.SplitLines(rewritten), FromFile: file.path, ToFile: file.path, Context: 3})
		if err != nil {
			return 0, fmt.Errorf("diff %s: %w", file.path, err)
		}
		fmt.Print(diff)
		if err := os.WriteFile(file.path, []byte(rewritten), file.mode); err != nil {
			return 0, fmt.Errorf("write %s: %w", file.path, err)
		}
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
