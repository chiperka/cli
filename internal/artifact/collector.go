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
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifacts directory: %w", err)
	}

	return &Collector{
		baseDir: baseDir,
	}, nil
}

// GetTestDir returns the artifact directory path for a test UUID.
// Structure: <baseDir>/tests/<testUUID>/artifacts/
func (c *Collector) GetTestDir(testUUID string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	dir := filepath.Join(c.baseDir, "tests", testUUID, "artifacts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create test artifact directory: %w", err)
	}

	return dir, nil
}

// SaveArtifact saves an artifact file for a test.
func (c *Collector) SaveArtifact(testUUID, filename string, content []byte) (string, error) {
	dir, err := c.GetTestDir(testUUID)
	if err != nil {
		return "", err
	}

	filename = filepath.Base(filename)

	filePath := filepath.Join(dir, filename)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write artifact: %w", err)
	}

	return filePath, nil
}

// SaveArtifactWithPath saves an artifact file preserving a relative path (creates intermediate dirs).
func (c *Collector) SaveArtifactWithPath(testUUID, relativePath string, content []byte) (string, error) {
	dir, err := c.GetTestDir(testUUID)
	if err != nil {
		return "", err
	}

	filePath := filepath.Join(dir, relativePath)
	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(dir)+string(os.PathSeparator)) {
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
func (c *Collector) ListArtifacts(testUUID string) ([]ArtifactInfo, error) {
	dir := filepath.Join(c.baseDir, "tests", testUUID, "artifacts")

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []ArtifactInfo{}, nil
	}

	var artifacts []ArtifactInfo
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
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
