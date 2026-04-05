// Package finder provides functionality for discovering test files.
package finder

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// TestFileSuffix is the required suffix for test definition files.
	TestFileSuffix = ".chiperka"
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

		// Check if file matches the pattern
		if strings.HasSuffix(info.Name(), TestFileSuffix) {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}
