package envfile

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// load parses a .env file and sets each key-value pair via os.Setenv.
// Keys present in protected are never overwritten (these are the original
// shell environment variables that should take precedence).
// Supports KEY=VALUE, KEY="VALUE", KEY='VALUE', comments (#), and blank lines.
func load(path string, protected map[string]struct{}) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open env file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, err := parseLine(line)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNum, err)
		}

		// Don't override variables that were set in the shell
		if _, ok := protected[key]; ok {
			continue
		}

		os.Setenv(key, value)
	}

	return scanner.Err()
}

// Load parses a .env file and sets each key-value pair via os.Setenv.
// Variables already present in the shell environment are not overwritten.
func Load(path string) error {
	return LoadAll([]string{path})
}

// LoadAll loads multiple .env files in order.
// Shell environment variables always take precedence.
// Among .env files, later files override earlier ones.
func LoadAll(paths []string) error {
	// Snapshot current environment so shell values are never overwritten
	protected := snapshotEnv()

	for _, path := range paths {
		if err := load(path, protected); err != nil {
			return err
		}
	}
	return nil
}

// snapshotEnv returns a set of all currently defined environment variable keys.
func snapshotEnv() map[string]struct{} {
	env := os.Environ()
	m := make(map[string]struct{}, len(env))
	for _, entry := range env {
		if k, _, ok := strings.Cut(entry, "="); ok {
			m[k] = struct{}{}
		}
	}
	return m
}

// parseLine extracts a key-value pair from a single line.
func parseLine(line string) (string, string, error) {
	// Split on first '='
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", "", fmt.Errorf("invalid line (no '=' found): %s", line)
	}

	key := strings.TrimSpace(line[:idx])
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}

	value := strings.TrimSpace(line[idx+1:])

	// Strip matching quotes
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}

	// Strip inline comment (only for unquoted values)
	rawValue := strings.TrimSpace(line[idx+1:])
	if len(rawValue) >= 2 && (rawValue[0] == '"' || rawValue[0] == '\'') {
		// Value was quoted — don't strip inline comments
	} else if i := strings.IndexByte(value, '#'); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}

	return key, value, nil
}
