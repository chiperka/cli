package cmd

import (
	"errors"
	"testing"

	"spark-cli/internal/model"
)

func TestValidateTest_Valid(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "valid-test",
		Services: []model.Service{
			{Name: "api", Image: "nginx:alpine"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d: %v", len(issues), issues)
	}
}

func TestValidateTest_NoServices(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "no-services",
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "no services defined")
}

func TestValidateTest_EmptyImage(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "empty-image",
		Services: []model.Service{
			{Name: "api"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "image is empty")
}

func TestValidateTest_BrokenRef(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "broken-ref",
		Services: []model.Service{
			{Ref: "nonexistent"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "not found")
}

func TestValidateTest_ValidRef(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "valid-ref",
		Services: []model.Service{
			{Ref: "postgres"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	templates := model.NewServiceTemplateCollection()
	templates.AddTemplate(&model.ServiceTemplate{
		Name:  "postgres",
		Image: "postgres:15",
	})

	issues := validateTest(test, suite, templates)

	for _, issue := range issues {
		if issue.Level == "error" {
			t.Errorf("unexpected error: %s", issue.Message)
		}
	}
}

func TestValidateTest_RefEmptyImage(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "ref-no-image",
		Services: []model.Service{
			{Ref: "broken"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	templates := model.NewServiceTemplateCollection()
	templates.AddTemplate(&model.ServiceTemplate{
		Name: "broken",
		// Image intentionally empty
	})

	issues := validateTest(test, suite, templates)

	assertHasIssue(t, issues, "error", "image is empty after resolving")
}

func TestValidateTest_MissingTarget(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "no-target",
		Services: []model.Service{
			{Name: "api", Image: "nginx:alpine"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "target is empty")
}

func TestValidateTest_MissingMethod(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "no-method",
		Services: []model.Service{
			{Name: "api", Image: "nginx:alpine"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "method is empty")
}

func TestValidateTest_CLIExecutor(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "valid-cli",
		Services: []model.Service{
			{Name: "app", Image: "myapp:latest"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorCLI,
			CLI: &model.CLICommand{
				Service: "app",
				Command: "echo hello",
			},
		},
		Assertions: []model.Assertion{
			{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d: %v", len(issues), issues)
	}
}

func TestValidateTest_CLIMissingConfig(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "cli-no-config",
		Services: []model.Service{
			{Name: "app", Image: "myapp:latest"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorCLI,
		},
		Assertions: []model.Assertion{
			{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "cli executor requires cli configuration")
}

func TestValidateTest_CLIMissingService(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "cli-no-service",
		Services: []model.Service{
			{Name: "app", Image: "myapp:latest"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorCLI,
			CLI: &model.CLICommand{
				Command: "echo hello",
			},
		},
		Assertions: []model.Assertion{
			{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "cli.service is empty")
}

func TestValidateTest_CLIMissingCommand(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "cli-no-command",
		Services: []model.Service{
			{Name: "app", Image: "myapp:latest"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorCLI,
			CLI: &model.CLICommand{
				Service: "app",
			},
		},
		Assertions: []model.Assertion{
			{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "cli.command is empty")
}

func TestValidateTest_UnknownExecutor(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "bad-executor",
		Services: []model.Service{
			{Name: "api", Image: "nginx:alpine"},
		},
		Execution: model.Execution{
			Executor: "grpc",
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "unknown executor type")
}

func TestValidateTest_NoAssertionsWarning(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Name: "no-assertions",
		Services: []model.Service{
			{Name: "api", Image: "nginx:alpine"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "warning", "no assertions defined")
	// Should be warning only, no errors
	for _, issue := range issues {
		if issue.Level == "error" {
			t.Errorf("unexpected error: %s", issue.Message)
		}
	}
}

func TestValidateTest_EmptyName(t *testing.T) {
	suite := model.Suite{Name: "Suite", FilePath: "test.spark"}
	test := model.Test{
		Services: []model.Service{
			{Name: "api", Image: "nginx:alpine"},
		},
		Execution: model.Execution{
			Executor: model.ExecutorHTTP,
			Target:   "http://api",
			Request:  model.HTTPRequest{Method: "GET", URL: "/"},
		},
		Assertions: []model.Assertion{
			{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		},
	}

	issues := validateTest(test, suite, model.NewServiceTemplateCollection())

	assertHasIssue(t, issues, "error", "test name is empty")
}

func TestExitError(t *testing.T) {
	err := exitErrorf(ExitTestFailure, "test failed: %d errors", 3)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatal("expected ExitError")
	}
	if exitErr.Code != ExitTestFailure {
		t.Errorf("expected code %d, got %d", ExitTestFailure, exitErr.Code)
	}
	if exitErr.Error() != "test failed: 3 errors" {
		t.Errorf("unexpected message: %s", exitErr.Error())
	}
}

func TestExitCodes(t *testing.T) {
	if ExitOK != 0 {
		t.Errorf("ExitOK should be 0, got %d", ExitOK)
	}
	if ExitTestFailure != 1 {
		t.Errorf("ExitTestFailure should be 1, got %d", ExitTestFailure)
	}
	if ExitInfraError != 2 {
		t.Errorf("ExitInfraError should be 2, got %d", ExitInfraError)
	}
	if ExitValidationError != 3 {
		t.Errorf("ExitValidationError should be 3, got %d", ExitValidationError)
	}
}

func TestHasErrors(t *testing.T) {
	if hasErrors(nil) {
		t.Error("nil should have no errors")
	}
	if hasErrors([]validationIssue{{Level: "warning"}}) {
		t.Error("warnings only should return false")
	}
	if !hasErrors([]validationIssue{{Level: "error"}}) {
		t.Error("errors should return true")
	}
}

func assertHasIssue(t *testing.T, issues []validationIssue, level, messageSubstring string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Level == level && contains(issue.Message, messageSubstring) {
			return
		}
	}
	t.Errorf("expected %s issue containing %q, got: %v", level, messageSubstring, issues)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchSubstring(s, substr)))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
