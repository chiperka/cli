package cloud

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRegistryAuthFromEnv(t *testing.T) {
	t.Setenv("CHIPERKA_REGISTRY_HOST", "registry.gitlab.com")
	t.Setenv("CHIPERKA_REGISTRY_USER", "gitlab-ci-token")
	t.Setenv("CHIPERKA_REGISTRY_PASSWORD", "job-token")

	got := RegistryAuthFromEnv()
	if len(got) != 1 || got[0] != (RegistryAuth{Host: "registry.gitlab.com", Username: "gitlab-ci-token", Password: "job-token"}) {
		t.Fatalf("unexpected registry auth: %+v", got)
	}

	body, _ := json.Marshal(&CreateRunRequest{RegistryAuth: got})
	if !strings.Contains(string(body), `"registry_auth":[{"host":"registry.gitlab.com"`) {
		t.Fatalf("registry_auth missing from payload: %s", body)
	}
}

func TestRegistryAuthFromEnv_Incomplete(t *testing.T) {
	t.Setenv("CHIPERKA_REGISTRY_HOST", "registry.gitlab.com")
	t.Setenv("CHIPERKA_REGISTRY_USER", "gitlab-ci-token")
	t.Setenv("CHIPERKA_REGISTRY_PASSWORD", "")

	if got := RegistryAuthFromEnv(); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
	body, _ := json.Marshal(&CreateRunRequest{RegistryAuth: RegistryAuthFromEnv()})
	if strings.Contains(string(body), "registry_auth") {
		t.Fatalf("empty registry_auth should be omitted: %s", body)
	}
}
