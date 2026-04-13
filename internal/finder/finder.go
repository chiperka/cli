// Package finder provides functionality for discovering test files.
package finder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// TestFileSuffix is the primary suffix for test definition files.
	TestFileSuffix = ".chiperka"
	// LegacyTestFileSuffix is the legacy suffix (still supported).
	LegacyTestFileSuffix = ".spark"
)

// Finder discovers test files in a directory tree.
type Finder struct {
	rootPath string
}

// New creates a new Finder for the given root directory.
func New(rootPath string) *Finder {
	return &Finder{
		rootPath: rootPath,
	}
}

// FindTestFiles walks the directory tree and returns all files matching *.chiperka pattern.
// Returns an error if the root directory cannot be accessed.
func (f *Finder) FindTestFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(f.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file matches either .chiperka or .spark pattern
		if strings.HasSuffix(info.Name(), TestFileSuffix) || strings.HasSuffix(info.Name(), LegacyTestFileSuffix) {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// FindAll discovers .chiperka files across multiple root paths, deduplicating
// results by absolute path. Each path is walked recursively.
func FindAll(paths []string) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string

	for _, root := range paths {
		f := New(root)
		found, err := f.FindTestFiles()
		if err != nil {
			return nil, fmt.Errorf("discovery path %s: %w", root, err)
		}
		for _, path := range found {
			abs, err := filepath.Abs(path)
			if err != nil {
				abs = path
			}
			if _, ok := seen[abs]; !ok {
				seen[abs] = struct{}{}
				files = append(files, path)
			}
		}
	}

	return files, nil
}
