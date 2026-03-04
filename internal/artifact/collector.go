// Package artifact handles test artifact collection and storage.
package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Collector manages artifact collection for test runs.
type Collector struct {
	baseDir string
	mu      sync.Mutex
}

// NewCollector creates a new artifact collector with the given base directory.
func NewCollector(baseDir string) (*Collector, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifacts directory: %w", err)
	}

	return &Collector{
		baseDir: baseDir,
	}, nil
}

// GetTestDir returns the artifact directory path for a test UUID.
// Creates the directory if it doesn't exist.
func (c *Collector) GetTestDir(uuid string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	testDir := filepath.Join(c.baseDir, uuid)
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create test artifact directory: %w", err)
	}

	return testDir, nil
}

// SaveArtifact saves an artifact file for a test.
// The filename should be flat (no subdirectories).
func (c *Collector) SaveArtifact(uuid, filename string, content []byte) (string, error) {
	testDir, err := c.GetTestDir(uuid)
	if err != nil {
		return "", err
	}

	// Ensure filename is flat (no path separators)
	filename = filepath.Base(filename)

	filePath := filepath.Join(testDir, filename)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write artifact: %w", err)
	}

	return filePath, nil
}

// SaveArtifactWithPath saves an artifact file preserving a relative path (creates intermediate dirs).
func (c *Collector) SaveArtifactWithPath(uuid, relativePath string, content []byte) (string, error) {
	testDir, err := c.GetTestDir(uuid)
	if err != nil {
		return "", err
	}

	filePath := filepath.Join(testDir, relativePath)
	// Prevent path traversal: resolved path must stay within testDir
	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(testDir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal detected: %s escapes artifact directory", relativePath)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create artifact subdirectory: %w", err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write artifact: %w", err)
	}

	return filePath, nil
}

// ListArtifacts returns a list of artifact files for a test UUID (recursive).
func (c *Collector) ListArtifacts(uuid string) ([]ArtifactInfo, error) {
	testDir := filepath.Join(c.baseDir, uuid)

	// If directory doesn't exist, return empty list
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		return []ArtifactInfo{}, nil
	}

	var artifacts []ArtifactInfo
	err := filepath.WalkDir(testDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip entries with errors
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(testDir, path)
		artifacts = append(artifacts, ArtifactInfo{
			Name: relPath,
			Path: path,
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk artifact directory: %w", err)
	}

	return artifacts, nil
}

// ArtifactInfo holds information about an artifact file.
type ArtifactInfo struct {
	Name string
	Path string
	Size int64
}

