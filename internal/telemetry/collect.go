package telemetry

import "chiperka-cli/internal/model"

// CollectRunStats extracts telemetry-relevant stats from a test collection.
// This inspects all suites/tests to detect which features are being used.
func CollectRunStats(tests *model.TestCollection, services *model.ServiceTemplateCollection) RunStats {
	var stats RunStats

	executors := map[model.ExecutorType]bool{}

	for _, suite := range tests.Suites {
		for _, test := range suite.Tests {
			// Services
			stats.ServiceCount += len(test.Services)

			// Executor type
			exec := test.Execution.Executor
			if exec == "" {
				exec = model.ExecutorHTTP
			}
			executors[exec] = true

			// Setup / Teardown
			if len(test.Setup) > 0 {
				stats.HasSetup = true
			}
			if len(test.Teardown) > 0 {
				stats.HasTeardown = true
			}

			// Hooks (from services)
			for _, svc := range test.Services {
				if len(svc.Hooks) > 0 {
					stats.HasHooks = true
				}
				if svc.Ref != "" {
					stats.HasServiceTemplates = true
				}
			}

			// Snapshots (check all assertion types)
			for _, a := range test.Assertions {
				if hasSnapshot(a) {
					stats.HasSnapshots = true
				}
			}
		}
	}

	// Also check service templates for hooks
	if services != nil && services.HasTemplates() {
		stats.HasServiceTemplates = true
	}

	// Determine executor type string
	switch {
	case executors[model.ExecutorHTTP] && executors[model.ExecutorCLI]:
		stats.ExecutorType = "mixed"
	case executors[model.ExecutorCLI]:
		stats.ExecutorType = "cli"
	default:
		stats.ExecutorType = "http"
	}

	return stats
}

// RunStats holds extracted feature usage data from a test collection.
type RunStats struct {
	ServiceCount      int
	ExecutorType      string
	HasSetup          bool
	HasTeardown       bool
	HasHooks          bool
	HasServiceTemplates bool
	HasSnapshots      bool
}

// hasSnapshot checks if any assertion in the test references a snapshot.
func hasSnapshot(a model.Assertion) bool {
	if a.Response != nil && a.Response.Body != nil && a.Response.Body.Snapshot != "" {
		return true
	}
	if a.CLI != nil {
		if a.CLI.Stdout != nil && a.CLI.Stdout.Snapshot != "" {
			return true
		}
		if a.CLI.Stderr != nil && a.CLI.Stderr.Snapshot != "" {
			return true
		}
	}
	if a.Artifact != nil && a.Artifact.Snapshot != "" {
		return true
	}
	return false
}
