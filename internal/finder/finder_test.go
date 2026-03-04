package finder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFinder_FindTestFiles_Simple(t *testing.T) {
	dir := t.TempDir()
	// Create .spark files
	for _, name := range []string{"test1.spark", "test2.spark"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("name: test"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	f := New(dir)
	files, err := f.FindTestFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestFinder_FindTestFiles_NestedDirs(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.spark"), []byte("name: root"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.spark"), []byte("name: nested"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	f := New(dir)
	files, err := f.FindTestFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestFinder_FindTestFiles_MixedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"test.spark", "readme.md", "config.yaml", "other.spark"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	f := New(dir)
	files, err := f.FindTestFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 .spark files, got %d", len(files))
	}
}

func TestFinder_FindTestFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	f := New(dir)
	files, err := f.FindTestFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestFinder_FindTestFiles_NonExistentPath(t *testing.T) {
	f := New("/nonexistent/path")
	_, err := f.FindTestFiles()
	if err == nil {
		t.Errorf("expected error for non-existent path")
	}
}

func TestFinder_FindTestFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.spark")
	if err := os.WriteFile(path, []byte("name: test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// When pointing at a directory containing a single file
	f := New(dir)
	files, err := f.FindTestFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestFinder_TestFileSuffix(t *testing.T) {
	if TestFileSuffix != ".spark" {
		t.Errorf("expected suffix '.spark', got %q", TestFileSuffix)
	}
}
