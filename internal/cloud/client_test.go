package cloud

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"spark-cli/internal/model"
)

func TestClient_HealthCheck_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected /health, got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if err := client.HealthCheck(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestClient_HealthCheck_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server down"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.HealthCheck()
	if err == nil {
		t.Errorf("expected error for 500 status")
	}
}

func TestClient_HealthCheck_Unreachable(t *testing.T) {
	client := NewClient("http://localhost:0")
	err := client.HealthCheck()
	if err == nil {
		t.Errorf("expected error for unreachable server")
	}
}

func TestClient_CreateRun_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runs" {
			t.Errorf("expected /api/runs, got %q", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %q", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type")
		}

		body, _ := io.ReadAll(r.Body)
		var req CreateRunRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("failed to unmarshal request: %v", err)
		}
		if len(req.Suites) != 1 {
			t.Errorf("expected 1 suite, got %d", len(req.Suites))
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateRunResponse{ID: "run-123"})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.CreateRun(&CreateRunRequest{
		Suites: []SuiteSubmission{
			{Name: "suite1", Tests: []model.Test{{Name: "t1"}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "run-123" {
		t.Errorf("expected run-123, got %q", resp.ID)
	}
}

func TestClient_CreateRun_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.CreateRun(&CreateRunRequest{})
	if err == nil {
		t.Errorf("expected error for 400 status")
	}
}

func TestClient_StopRun_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runs/run-123/stop" {
			t.Errorf("expected /api/runs/run-123/stop, got %q", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %q", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if err := client.StopRun("run-123"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClient_StopRun_Unreachable(t *testing.T) {
	client := NewClient("http://localhost:0")
	err := client.StopRun("run-123")
	if err == nil {
		t.Errorf("expected error for unreachable server")
	}
}

// --- BuildSubmission ---

func TestClient_BuildSubmission_Basic(t *testing.T) {
	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name:     "suite1",
		FilePath: "tests/suite1.spark",
		Tests: []model.Test{
			{Name: "t1"},
			{Name: "t2"},
		},
	})

	services := model.NewServiceTemplateCollection()

	req, err := BuildSubmission(tests, services, "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Config.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", req.Config.Version)
	}
	if len(req.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(req.Suites))
	}
	if len(req.Suites[0].Tests) != 2 {
		t.Errorf("expected 2 tests, got %d", len(req.Suites[0].Tests))
	}
}

func TestClient_BuildSubmission_ResolvesServiceRefs(t *testing.T) {
	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name: "suite1",
		Tests: []model.Test{
			{
				Name: "t1",
				Services: []model.Service{
					{Ref: "db"},
				},
			},
		},
	})

	services := model.NewServiceTemplateCollection()
	services.AddTemplate(&model.ServiceTemplate{
		Name:  "db",
		Image: "postgres:15",
	})

	req, err := BuildSubmission(tests, services, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Suites[0].Tests[0].Services[0].Image != "postgres:15" {
		t.Errorf("expected resolved image, got %q", req.Suites[0].Tests[0].Services[0].Image)
	}
}

func TestClient_BuildSubmission_UnresolvedRef(t *testing.T) {
	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name: "suite1",
		Tests: []model.Test{
			{
				Name: "t1",
				Services: []model.Service{
					{Ref: "nonexistent"},
				},
			},
		},
	})

	services := model.NewServiceTemplateCollection()

	_, err := BuildSubmission(tests, services, "")
	if err == nil {
		t.Errorf("expected error for unresolved service ref")
	}
}

func TestClient_BuildSubmission_Empty(t *testing.T) {
	tests := model.NewTestCollection()
	services := model.NewServiceTemplateCollection()

	req, err := BuildSubmission(tests, services, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Suites) != 0 {
		t.Errorf("expected 0 suites, got %d", len(req.Suites))
	}
}
