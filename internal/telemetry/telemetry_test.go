package telemetry

import (
	"fmt"
	"os"
	"testing"
)

func TestIsDisabled_DoNotTrack(t *testing.T) {
	os.Setenv("DO_NOT_TRACK", "1")
	defer os.Unsetenv("DO_NOT_TRACK")

	if !IsDisabled() {
		t.Error("expected telemetry to be disabled when DO_NOT_TRACK=1")
	}
}

func TestIsDisabled_NotSet(t *testing.T) {
	os.Unsetenv("DO_NOT_TRACK")
	// No config file — IsDisabled returns false (default enabled)
	if IsDisabled() {
		t.Error("expected telemetry to be enabled by default")
	}
}

func TestDetectCIProvider(t *testing.T) {
	tests := []struct {
		env      string
		value    string
		expected string
	}{
		{"GITHUB_ACTIONS", "true", "github_actions"},
		{"GITLAB_CI", "true", "gitlab_ci"},
		{"CIRCLECI", "true", "circleci"},
		{"TRAVIS", "true", "travis"},
		{"JENKINS_URL", "http://jenkins", "jenkins"},
		{"CODEBUILD_BUILD_ID", "id", "codebuild"},
		{"TF_BUILD", "True", "azure_devops"},
		{"BITBUCKET_PIPELINE", "true", "bitbucket"},
		{"BUILDKITE", "true", "buildkite"},
		{"DRONE", "true", "drone"},
		{"TEAMCITY_VERSION", "2024.1", "teamcity"},
		{"CI", "true", "unknown_ci"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			// Clear all CI vars
			for _, tc := range tests {
				os.Unsetenv(tc.env)
			}

			os.Setenv(tt.env, tt.value)
			defer os.Unsetenv(tt.env)

			got := detectCIProvider()
			if got != tt.expected {
				t.Errorf("detectCIProvider() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDetectCIProvider_None(t *testing.T) {
	ciVars := []string{"GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI", "TRAVIS", "JENKINS_URL",
		"CODEBUILD_BUILD_ID", "TF_BUILD", "BITBUCKET_PIPELINE", "BUILDKITE", "DRONE", "TEAMCITY_VERSION", "CI"}
	for _, v := range ciVars {
		os.Unsetenv(v)
	}

	got := detectCIProvider()
	if got != "" {
		t.Errorf("detectCIProvider() = %q, want empty", got)
	}
}

func TestDetectRuntimeEnv_Native(t *testing.T) {
	os.Unsetenv("COMPOSE_PROJECT_NAME")
	// Can't easily test docker detection in unit tests,
	// but on the host it should be "native"
	got := detectRuntimeEnv()
	if got != "native" && got != "docker" {
		t.Errorf("detectRuntimeEnv() = %q, want native or docker", got)
	}
}

func TestDetectRuntimeEnv_DockerCompose(t *testing.T) {
	os.Setenv("COMPOSE_PROJECT_NAME", "myproject")
	defer os.Unsetenv("COMPOSE_PROJECT_NAME")

	got := detectRuntimeEnv()
	if got != "docker_compose" {
		t.Errorf("detectRuntimeEnv() = %q, want docker_compose", got)
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		msg      string
		expected string
	}{
		{"docker daemon not running", "docker"},
		{"failed to parse YAML", "parse_error"},
		{"context deadline exceeded", "timeout"},
		{"no *.chiperka files found", "no_test_files"},
		{"connection refused", "network"},
		{"permission denied", "permission"},
		{"file not found", "not_found"},
		{"cloud API error", "cloud"},
		{"something weird happened", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := ClassifyError(fmt.Errorf(tt.msg))
			if got != tt.expected {
				t.Errorf("ClassifyError(%q) = %q, want %q", tt.msg, got, tt.expected)
			}
		})
	}
}

func TestClassifyError_Nil(t *testing.T) {
	if got := ClassifyError(nil); got != "unknown" {
		t.Errorf("ClassifyError(nil) = %q, want unknown", got)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	if len(id1) != 32 {
		t.Errorf("generateID() length = %d, want 32", len(id1))
	}
	if id1 == id2 {
		t.Error("generateID() should return unique values")
	}
}

func TestHashVisitor(t *testing.T) {
	h1 := hashVisitor("1.2.3.4", "chiperka-cli/1.0")
	h2 := hashVisitor("1.2.3.4", "chiperka-cli/1.0")
	h3 := hashVisitor("5.6.7.8", "chiperka-cli/1.0")
	h4 := hashVisitor("1.2.3.4", "chiperka-cli/2.0")

	// Same input = same hash
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}

	// Different IP = different hash
	if h1 == h3 {
		t.Error("different IP should produce different hash")
	}

	// Different UA = different hash
	if h1 == h4 {
		t.Error("different UA should produce different hash")
	}

	// Hash is 64 chars (sha256 hex)
	if len(h1) != 64 {
		t.Errorf("hash length = %d, want 64", len(h1))
	}
}
