package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadBasicKeyValue(t *testing.T) {
	path := writeEnvFile(t, "FOO=bar\nBAZ=qux\n")
	os.Unsetenv("FOO")
	os.Unsetenv("BAZ")

	if err := Load(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("FOO"); got != "bar" {
		t.Errorf("FOO = %q, want %q", got, "bar")
	}
	if got := os.Getenv("BAZ"); got != "qux" {
		t.Errorf("BAZ = %q, want %q", got, "qux")
	}
}

func TestLoadQuotedValues(t *testing.T) {
	path := writeEnvFile(t, `DOUBLE="hello world"
SINGLE='single quoted'
`)
	os.Unsetenv("DOUBLE")
	os.Unsetenv("SINGLE")

	if err := Load(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("DOUBLE"); got != "hello world" {
		t.Errorf("DOUBLE = %q, want %q", got, "hello world")
	}
	if got := os.Getenv("SINGLE"); got != "single quoted" {
		t.Errorf("SINGLE = %q, want %q", got, "single quoted")
	}
}

func TestLoadCommentsAndBlankLines(t *testing.T) {
	path := writeEnvFile(t, `# This is a comment
KEY1=value1

  # Indented comment

KEY2=value2
`)
	os.Unsetenv("KEY1")
	os.Unsetenv("KEY2")

	if err := Load(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("KEY1"); got != "value1" {
		t.Errorf("KEY1 = %q, want %q", got, "value1")
	}
	if got := os.Getenv("KEY2"); got != "value2" {
		t.Errorf("KEY2 = %q, want %q", got, "value2")
	}
}

func TestLoadInlineComment(t *testing.T) {
	path := writeEnvFile(t, "KEY=value # this is a comment\n")
	os.Unsetenv("KEY")

	if err := Load(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("KEY"); got != "value" {
		t.Errorf("KEY = %q, want %q", got, "value")
	}
}

func TestLoadEmptyValue(t *testing.T) {
	path := writeEnvFile(t, "EMPTY=\n")
	os.Unsetenv("EMPTY")

	if err := Load(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("EMPTY"); got != "" {
		t.Errorf("EMPTY = %q, want %q", got, "")
	}
}

func TestLoadMissingFile(t *testing.T) {
	err := Load("/nonexistent/.env")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidLine(t *testing.T) {
	path := writeEnvFile(t, "NOEQUALS\n")

	err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid line")
	}
}

func TestLoadAllLaterFileOverridesEarlier(t *testing.T) {
	file1 := writeEnvFile(t, "LOADALL_KEY=first\n")
	dir := t.TempDir()
	file2 := filepath.Join(dir, ".env.local")
	if err := os.WriteFile(file2, []byte("LOADALL_KEY=second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("LOADALL_KEY")

	if err := LoadAll([]string{file1, file2}); err != nil {
		t.Fatal(err)
	}

	// Later file overrides earlier file
	if got := os.Getenv("LOADALL_KEY"); got != "second" {
		t.Errorf("LOADALL_KEY = %q, want %q", got, "second")
	}
}

func TestLoadDoesNotOverrideExistingEnv(t *testing.T) {
	os.Setenv("EXISTING_VAR", "from_shell")
	defer os.Unsetenv("EXISTING_VAR")

	path := writeEnvFile(t, "EXISTING_VAR=from_file\n")

	if err := Load(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("EXISTING_VAR"); got != "from_shell" {
		t.Errorf("EXISTING_VAR = %q, want %q (shell should take precedence)", got, "from_shell")
	}
}

func TestLoadSpacesAroundEquals(t *testing.T) {
	path := writeEnvFile(t, "  KEY  =  value  \n")
	os.Unsetenv("KEY")

	if err := Load(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("KEY"); got != "value" {
		t.Errorf("KEY = %q, want %q", got, "value")
	}
}
