package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"chiperka-cli/internal/result"

	"github.com/spf13/cobra"
)

var resultCmd = &cobra.Command{
	Use:   "result",
	Short: "Inspect stored test results",
	Long:  "Browse test results with progressive disclosure. Each level shows just enough detail to decide whether to drill deeper.",
}

var resultRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "List recent test runs",
	RunE:  listRuns,
}

var resultRunCmd = &cobra.Command{
	Use:   "run <run-uuid>",
	Short: "Show run summary with test list",
	Args:  cobra.ExactArgs(1),
	RunE:  showRun,
}

var resultTestCmd = &cobra.Command{
	Use:   "test <test-uuid>",
	Short: "Show test detail with artifacts",
	Args:  cobra.ExactArgs(1),
	RunE:  showTest,
}

var resultArtifactCmd = &cobra.Command{
	Use:   "artifact <test-uuid> <filename>",
	Short: "Output raw artifact content",
	Args:  cobra.ExactArgs(2),
	RunE:  showArtifact,
}

var (
	resultLimit int
	resultJSON  bool
)

func init() {
	rootCmd.AddCommand(resultCmd)
	resultCmd.AddCommand(resultRunsCmd, resultRunCmd, resultTestCmd, resultArtifactCmd)

	resultRunsCmd.Flags().IntVar(&resultLimit, "limit", 20, "Maximum number of runs to show")
	resultRunsCmd.Flags().BoolVar(&resultJSON, "json", false, "Output as JSON")
	resultRunCmd.Flags().BoolVar(&resultJSON, "json", false, "Output as JSON")
	resultTestCmd.Flags().BoolVar(&resultJSON, "json", false, "Output as JSON")
}

func storeForUUID(uuid string) result.Store {
	if result.IsCloud(uuid) {
		return result.NewCloudStore()
	}
	return result.DefaultLocalStore()
}

func listRuns(cmd *cobra.Command, args []string) error {
	store := result.DefaultLocalStore()
	runs, err := store.ListRuns(resultLimit)
	if err != nil {
		return exitErrorf(ExitInfraError, "%v", err)
	}

	if len(runs) == 0 {
		fmt.Println("No results stored yet.")
		return nil
	}

	if resultJSON {
		return outputJSON(runs)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "UUID\tSTATUS\tTOTAL\tPASSED\tFAILED\tDURATION")
	for _, r := range runs {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%dms\n",
			r.UUID, r.Status, r.Total, r.Passed, r.Failed, r.Duration)
	}
	w.Flush()
	return nil
}

func showRun(cmd *cobra.Command, args []string) error {
	uuid := args[0]
	store := storeForUUID(uuid)
	run, err := store.GetRun(uuid)
	if err != nil {
		return exitErrorf(ExitInfraError, "%v", err)
	}

	if resultJSON {
		return outputJSON(run)
	}

	fmt.Printf("Run: %s\n", run.UUID)
	fmt.Printf("Status: %s\n", run.Status)
	fmt.Printf("Duration: %dms\n", run.Duration)
	fmt.Printf("Tests: %d passed, %d failed, %d errored, %d skipped (%d total)\n\n",
		run.Passed, run.Failed, run.Errored, run.Skipped, run.Total)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "UUID\tSTATUS\tNAME\tSUITE\tDURATION")
	for _, t := range run.Tests {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%dms\n",
			t.UUID, t.Status, t.Name, t.Suite, t.Duration)
	}
	w.Flush()
	return nil
}

func showTest(cmd *cobra.Command, args []string) error {
	uuid := args[0]
	store := storeForUUID(uuid)
	test, err := store.GetTest(uuid)
	if err != nil {
		return exitErrorf(ExitInfraError, "%v", err)
	}

	if resultJSON {
		return outputJSON(test)
	}

	fmt.Printf("Test: %s\n", test.Name)
	fmt.Printf("Suite: %s\n", test.Suite)
	fmt.Printf("UUID: %s\n", test.UUID)
	fmt.Printf("Status: %s\n", test.Status)
	fmt.Printf("Duration: %dms\n", test.Duration)

	if test.Error != "" {
		fmt.Printf("Error: %s\n", test.Error)
	}

	if len(test.Assertions) > 0 {
		fmt.Println("\nAssertions:")
		for _, a := range test.Assertions {
			mark := "✓"
			if !a.Passed {
				mark = "✗"
			}
			fmt.Printf("  %s %s", mark, a.Message)
			if !a.Passed {
				fmt.Printf(" (expected: %s, actual: %s)", a.Expected, a.Actual)
			}
			fmt.Println()
		}
	}

	if len(test.Services) > 0 {
		fmt.Println("\nServices:")
		for _, s := range test.Services {
			fmt.Printf("  %s (%s) — %dms\n", s.Name, s.Image, s.Duration)
		}
	}

	if len(test.HTTPExchanges) > 0 {
		fmt.Println("\nHTTP Exchanges:")
		for _, h := range test.HTTPExchanges {
			fmt.Printf("  [%s] %s %s → %d (%dms)\n",
				h.Phase, h.Method, h.URL, h.StatusCode, h.Duration)
		}
	}

	if len(test.CLIExecutions) > 0 {
		fmt.Println("\nCLI Executions:")
		for _, c := range test.CLIExecutions {
			fmt.Printf("  [%s] %s: %s → exit %d (%dms)\n",
				c.Phase, c.Service, c.Command, c.ExitCode, c.Duration)
		}
	}

	if len(test.Artifacts) > 0 {
		fmt.Println("\nArtifacts:")
		for _, a := range test.Artifacts {
			fmt.Printf("  %s (%d bytes)\n", a.Name, a.Size)
		}
	}

	return nil
}

func showArtifact(cmd *cobra.Command, args []string) error {
	testUUID := args[0]
	name := args[1]
	store := storeForUUID(testUUID)
	content, err := store.GetArtifact(testUUID, name)
	if err != nil {
		return exitErrorf(ExitInfraError, "%v", err)
	}

	os.Stdout.Write(content)
	return nil
}

func outputJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
