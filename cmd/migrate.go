package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var migrateDryRun bool

var migrateCmd = &cobra.Command{
	Use:   "migrate <path>",
	Short: "Migrate kindless .chiperka files to the kind-aware format",
	Long: `Migrate walks <path> recursively and prepends "kind: test" to every
.chiperka / .spark file that does not already declare a top-level kind.

The command is idempotent: files that already declare a kind are left
untouched. Use --dry-run to preview the changes without writing files.`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Print planned changes without writing files")
}

// migrationStats summarises what migrate did (or would do).
type migrationStats struct {
	filesScanned int
	filesUpdated int
}

func runMigrate(cmd *cobra.Command, args []string) error {
	root := args[0]
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", root, err)
	}

	stats := &migrationStats{}

	if info.IsDir() {
		err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			return migrateFile(path, stats)
		})
	} else {
		err = migrateFile(root, stats)
	}
	if err != nil {
		return err
	}

	prefix := ""
	if migrateDryRun {
		prefix = "[dry-run] "
	}
	fmt.Printf("%sMigration summary:\n", prefix)
	fmt.Printf("  files scanned : %d\n", stats.filesScanned)
	fmt.Printf("  files updated : %d\n", stats.filesUpdated)
	return nil
}

// migrateFile dispatches by suffix and falls through silently for anything
// that is not a .chiperka / .spark file.
func migrateFile(path string, stats *migrationStats) error {
	base := filepath.Base(path)
	if !strings.HasSuffix(base, ".chiperka") && !strings.HasSuffix(base, ".spark") {
		return nil
	}
	return migrateKindlessTestFile(path, stats)
}

// migrateKindlessTestFile prepends "kind: test" to a .chiperka/.spark file
// that does not already declare a top-level kind. Idempotent.
func migrateKindlessTestFile(path string, stats *migrationStats) error {
	stats.filesScanned++

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	hasKind, err := topLevelHasKind(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if hasKind {
		return nil // idempotent: already has kind
	}

	newContent := append([]byte("kind: test\n"), data...)

	if migrateDryRun {
		fmt.Printf("[dry-run] would prepend `kind: test` to %s\n", path)
		stats.filesUpdated++
		return nil
	}

	if err := os.WriteFile(path, newContent, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("updated %s (added kind: test)\n", path)
	stats.filesUpdated++
	return nil
}

// topLevelHasKind reports whether a YAML document has a top-level `kind:` key.
func topLevelHasKind(data []byte) (bool, error) {
	var top map[string]interface{}
	if err := yaml.Unmarshal(data, &top); err != nil {
		return false, err
	}
	_, ok := top["kind"]
	return ok, nil
}
