package telemetry

import (
	"testing"

	"chiperka-cli/internal/model"
)

func TestCollectRunStats_Empty(t *testing.T) {
	tests := &model.TestCollection{}
	stats := CollectRunStats(tests, nil)

	if stats.ServiceCount != 0 {
		t.Errorf("ServiceCount = %d, want 0", stats.ServiceCount)
	}
	if stats.ExecutorType != "http" {
		t.Errorf("ExecutorType = %q, want http", stats.ExecutorType)
	}
	if stats.HasSetup || stats.HasTeardown || stats.HasHooks || stats.HasSnapshots || stats.HasServiceTemplates {
		t.Error("all feature flags should be false for empty collection")
	}
}

func TestCollectRunStats_HTTPExecutor(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{
						Execution: model.Execution{Executor: model.ExecutorHTTP},
						Services:  []model.Service{{Name: "api", Image: "nginx"}},
					},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if stats.ExecutorType != "http" {
		t.Errorf("ExecutorType = %q, want http", stats.ExecutorType)
	}
	if stats.ServiceCount != 1 {
		t.Errorf("ServiceCount = %d, want 1", stats.ServiceCount)
	}
}

func TestCollectRunStats_CLIExecutor(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{Execution: model.Execution{Executor: model.ExecutorCLI}},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if stats.ExecutorType != "cli" {
		t.Errorf("ExecutorType = %q, want cli", stats.ExecutorType)
	}
}

func TestCollectRunStats_MixedExecutor(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{Execution: model.Execution{Executor: model.ExecutorHTTP}},
					{Execution: model.Execution{Executor: model.ExecutorCLI}},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if stats.ExecutorType != "mixed" {
		t.Errorf("ExecutorType = %q, want mixed", stats.ExecutorType)
	}
}

func TestCollectRunStats_Setup(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{Setup: []model.SetupInstruction{{}}},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if !stats.HasSetup {
		t.Error("HasSetup should be true")
	}
}

func TestCollectRunStats_Teardown(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{Teardown: []model.SetupInstruction{{}}},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if !stats.HasTeardown {
		t.Error("HasTeardown should be true")
	}
}

func TestCollectRunStats_Hooks(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{
						Services: []model.Service{
							{Hooks: []model.Hook{{Slot: "beforeExecution"}}},
						},
					},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if !stats.HasHooks {
		t.Error("HasHooks should be true")
	}
}

func TestCollectRunStats_ServiceTemplates(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{Services: []model.Service{{Ref: "postgres"}}},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if !stats.HasServiceTemplates {
		t.Error("HasServiceTemplates should be true when service has ref")
	}
}

func TestCollectRunStats_Snapshots(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{
						Assertions: []model.Assertion{
							{Response: &model.ResponseAssertion{Body: &model.ResponseBodyAssertion{Snapshot: "snap.json"}}},
						},
					},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if !stats.HasSnapshots {
		t.Error("HasSnapshots should be true")
	}
}

func TestCollectRunStats_MultipleServices(t *testing.T) {
	tests := &model.TestCollection{
		Suites: []model.Suite{
			{
				Tests: []model.Test{
					{
						Services: []model.Service{
							{Name: "db", Image: "postgres"},
							{Name: "api", Image: "nginx"},
						},
					},
					{
						Services: []model.Service{
							{Name: "redis", Image: "redis"},
						},
					},
				},
			},
		},
	}

	stats := CollectRunStats(tests, nil)
	if stats.ServiceCount != 3 {
		t.Errorf("ServiceCount = %d, want 3", stats.ServiceCount)
	}
}
